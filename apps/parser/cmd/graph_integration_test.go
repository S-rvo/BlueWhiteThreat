//go:build integration

package main

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/db"
	"github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/entities"
)

func TestGraphProjectionRetriesPersistedEntities(t *testing.T) {
	uri := os.Getenv("BWT_PARSER_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("set BWT_PARSER_TEST_MONGO_URI to a disposable, empty MongoDB instance")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	coll := client.Database("BlueWhiteThreat").Collection("article")
	ids := make([]string, 6)
	for i := range ids {
		ids[i] = primitive.NewObjectID().Hex()
	}
	first, legacy, empty, uncertain, negative, unclassified := ids[0], ids[1], ids[2], ids[3], ids[4], ids[5]
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := coll.DeleteMany(ctx, bson.M{"_id": bson.M{"$in": ids}}); err != nil {
			t.Error(err)
		}
		if err := client.Disconnect(ctx); err != nil {
			t.Error(err)
		}
	})

	ents := entities.Entities{CVEs: []string{"CVE-2024-1234"}}
	fixture := db.UnparsedArticle{ID: first, Content: "Synthetic document."}
	relevance := bson.M{"relevant": true}
	if _, err := coll.InsertOne(ctx, bson.M{"_id": first, "content": fixture.Content, "relevance": relevance}); err != nil {
		t.Fatal(err)
	}
	if _, err := coll.InsertMany(ctx, []any{
		bson.M{"_id": uncertain, "content": fixture.Content, "relevance": bson.M{"relevant": true, "needs_review": true}},
		bson.M{"_id": negative, "content": fixture.Content, "relevance": bson.M{"relevant": false}, "entities": ents},
		bson.M{"_id": unclassified, "content": fixture.Content},
	}); err != nil {
		t.Fatal(err)
	}
	unparsed, err := db.FetchUnparsedArticles(ctx, client, batchSize)
	if err != nil || len(unparsed) != 1 || unparsed[0].ID != first {
		t.Fatal("only a confident positive should enter extraction")
	}
	if err := db.SaveEntities(ctx, client, db.UnparsedArticle{ID: first, Content: "old revision"}, ents); err != nil {
		t.Fatal(err)
	}
	if n, err := coll.CountDocuments(ctx, bson.M{"_id": first, "entities": bson.M{"$exists": true}}); err != nil || n != 0 {
		t.Fatal("an extraction from a stale text revision was persisted")
	}
	if err := db.SaveEntities(ctx, client, fixture, ents); err != nil {
		t.Fatal(err)
	}

	var attempts []string
	fail := true
	write := func(_ context.Context, id string, got entities.Entities) error {
		attempts = append(attempts, id)
		if id == empty {
			if !got.IsEmpty() {
				t.Errorf("empty article projected with entities %v", got)
			}
		} else if !reflect.DeepEqual(got.CVEs, ents.CVEs) {
			t.Errorf("projected CVEs = %v; want %v", got.CVEs, ents.CVEs)
		}
		if fail {
			return errors.New("temporary graph failure")
		}
		return nil
	}
	projectPending(ctx, client, write)
	pending, err := db.FetchPendingGraphArticles(ctx, client, batchSize)
	if err != nil || len(pending) != 1 || pending[0].ID != first {
		t.Fatalf("after graph failure, pending = %v, err = %v", pending, err)
	}
	unparsed, err = db.FetchUnparsedArticles(ctx, client, batchSize)
	if err != nil || len(unparsed) != 0 {
		t.Fatalf("extraction should not be repeated: unparsed = %v, err = %v", unparsed, err)
	}
	if err := db.SaveEntities(ctx, client, fixture, entities.Entities{}); err != nil {
		t.Fatal(err)
	}

	if _, err := coll.InsertMany(ctx, []any{
		bson.M{"_id": legacy, "entities": ents, "relevance": relevance},
		bson.M{"_id": empty, "entities": entities.Entities{}, "relevance": relevance},
	}); err != nil {
		t.Fatal(err)
	}
	fail = false
	projectPending(ctx, client, write)
	projectPending(ctx, client, write)
	// Une extraction vide est aussi écrite : elle retire du graphe les entités
	// d'une révision précédente de l'article.
	if want := []string{first, legacy, empty, first}; !reflect.DeepEqual(attempts, want) {
		t.Fatalf("graph attempts = %v; want %v (legacy, empty, retry, no repeat)", attempts, want)
	}
	pending, err = db.FetchPendingGraphArticles(ctx, client, batchSize)
	if err != nil || len(pending) != 0 {
		t.Fatalf("after graph recovery, pending = %v, err = %v", pending, err)
	}
}
