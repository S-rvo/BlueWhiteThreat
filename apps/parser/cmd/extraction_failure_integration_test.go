//go:build integration

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/db"
	"github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/extractor"
)

// Un extracteur en panne ne doit ni bloquer la file (poison pill) ni la
// parcourir entièrement : chaque échec est enregistré avec un backoff, et le
// cycle s'interrompt après maxConsecutiveExtractionFailures échecs.
func TestExtractionFailuresBackOffAndTripCircuitBreaker(t *testing.T) {
	uri := os.Getenv("BWT_PARSER_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("set BWT_PARSER_TEST_MONGO_URI to a disposable, empty MongoDB instance")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	coll := client.Database("BlueWhiteThreat").Collection("article")
	ids := make([]string, maxConsecutiveExtractionFailures+1)
	docs := make([]any, len(ids))
	for i := range ids {
		ids[i] = primitive.NewObjectID().Hex() // croissants : ordre de traitement connu
		docs[i] = bson.M{"_id": ids[i], "content": "Synthetic document " + ids[i], "relevance": bson.M{"relevant": true}}
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := coll.DeleteMany(ctx, bson.M{"_id": bson.M{"$in": ids}}); err != nil {
			t.Error(err)
		}
		_ = client.Disconnect(ctx)
	})
	if _, err := coll.InsertMany(ctx, docs); err != nil {
		t.Fatal(err)
	}
	if pending, err := db.FetchUnparsedArticles(ctx, client, batchSize); err != nil || len(pending) != len(ids) {
		t.Fatalf("database is not empty or fixtures not eligible: %d articles, err=%v", len(pending), err)
	}

	var calls atomic.Int32
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.Error(w, "model unavailable", http.StatusInternalServerError)
	}))
	defer ollama.Close()
	broken := extractor.NewOllamaExtractor(ollama.URL, "fixture")

	before := time.Now()
	extractPending(ctx, client, broken, nil)
	if got := int(calls.Load()); got != maxConsecutiveExtractionFailures {
		t.Fatalf("extractor called %d times; circuit breaker should stop at %d", got, maxConsecutiveExtractionFailures)
	}
	for i, id := range ids {
		var doc struct {
			Failures  int       `bson:"extraction_failures"`
			RetryAt   time.Time `bson:"extraction_retry_at"`
			LastError string    `bson:"extraction_last_error"`
		}
		if err := coll.FindOne(ctx, bson.M{"_id": id}).Decode(&doc); err != nil {
			t.Fatal(err)
		}
		if i == len(ids)-1 {
			if doc.Failures != 0 || !doc.RetryAt.IsZero() {
				t.Fatalf("article after the breaker was touched: %+v", doc)
			}
			continue
		}
		if doc.Failures != 1 || doc.LastError == "" ||
			doc.RetryAt.Before(before.Add(db.ExtractionBackoff(1)-time.Second)) {
			t.Fatalf("failure %d not recorded with backoff: %+v", i, doc)
		}
	}

	// Les articles en backoff sortent de la file : seul le dernier reste.
	pending, err := db.FetchUnparsedArticles(ctx, client, batchSize)
	if err != nil || len(pending) != 1 || pending[0].ID != ids[len(ids)-1] {
		t.Fatalf("backed-off articles still queued: %v, err=%v", pending, err)
	}

	// Échéance passée : l'article revient et le compteur continue de croître.
	if _, err := coll.UpdateByID(ctx, ids[0], bson.M{"$set": bson.M{"extraction_retry_at": time.Now().Add(-time.Minute)}}); err != nil {
		t.Fatal(err)
	}
	extractPending(ctx, client, broken, nil)
	var doc struct {
		Failures int       `bson:"extraction_failures"`
		RetryAt  time.Time `bson:"extraction_retry_at"`
	}
	if err := coll.FindOne(ctx, bson.M{"_id": ids[0]}).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if doc.Failures != 2 || time.Until(doc.RetryAt) < db.ExtractionBackoff(2)-time.Minute {
		t.Fatalf("second failure not backed off further: %+v", doc)
	}
}
