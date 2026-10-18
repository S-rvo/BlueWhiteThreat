//go:build integration

package db

import (
	"context"
	"testing"

	"github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/entities"
)

func TestSaveAndFetchEntities(t *testing.T) {
	ctx := context.Background()
	client, err := ConnectMongo(ctx)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Disconnect(ctx)

	coll := client.Database("BlueWhiteThreat").Collection("article")
	res, err := coll.InsertOne(ctx, map[string]any{"content": "test article"})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	ents := entities.Entities{}
	ents.CVEs = []string{"CVE-2024-1111"}
	if err := SaveEntities(ctx, client, res.InsertedID, ents); err != nil {
		t.Fatalf("save: %v", err)
	}

	unparsed, err := FetchUnparsedArticles(ctx, client, 10)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	for _, a := range unparsed {
		if a.ID == res.InsertedID {
			t.Fatal("article should no longer be unparsed after SaveEntities")
		}
	}
}
