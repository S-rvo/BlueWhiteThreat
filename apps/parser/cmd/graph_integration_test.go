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
	first, legacy, empty := primitive.NewObjectID().Hex(), primitive.NewObjectID().Hex(), primitive.NewObjectID().Hex()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := coll.DeleteMany(ctx, bson.M{"_id": bson.M{"$in": []string{first, legacy, empty}}}); err != nil {
			t.Error(err)
		}
		if err := client.Disconnect(ctx); err != nil {
			t.Error(err)
		}
	})

	ents := entities.Entities{CVEs: []string{"CVE-2024-1234"}}
	if _, err := coll.InsertOne(ctx, bson.M{"_id": first, "content": "Synthetic document."}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveEntities(ctx, client, first, ents); err != nil {
		t.Fatal(err)
	}

	var attempts []string
	fail := true
	write := func(_ context.Context, id string, got entities.Entities) error {
		attempts = append(attempts, id)
		if !reflect.DeepEqual(got.CVEs, ents.CVEs) {
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
	unparsed, err := db.FetchUnparsedArticles(ctx, client, batchSize)
	if err != nil || len(unparsed) != 0 {
		t.Fatalf("extraction should not be repeated: unparsed = %v, err = %v", unparsed, err)
	}
	if err := db.SaveEntities(ctx, client, first, entities.Entities{}); err != nil {
		t.Fatal(err)
	}

	if _, err := coll.InsertMany(ctx, []any{
		bson.M{"_id": legacy, "entities": ents},
		bson.M{"_id": empty, "entities": entities.Entities{}},
	}); err != nil {
		t.Fatal(err)
	}
	fail = false
	projectPending(ctx, client, write)
	projectPending(ctx, client, write)
	if want := []string{first, legacy, first}; !reflect.DeepEqual(attempts, want) {
		t.Fatalf("graph attempts = %v; want %v (legacy, retry, no repeat or empty write)", attempts, want)
	}
	pending, err = db.FetchPendingGraphArticles(ctx, client, batchSize)
	if err != nil || len(pending) != 0 {
		t.Fatalf("after graph recovery, pending = %v, err = %v", pending, err)
	}
}
