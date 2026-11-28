//go:build integration

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/db"
	"github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/entities"
	"github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/extractor"
)

func TestSmokeExtractAndReplay(t *testing.T) {
	if os.Getenv("BWT_DISPOSABLE_PIPELINE") != "1" {
		t.Skip("run tests/pipeline/smoke.py against disposable databases")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	client, err := db.ConnectMongo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Disconnect(ctx) }()
	coll := client.Database("BlueWhiteThreat").Collection("article")
	uri, password := os.Getenv("NEO4J_URI"), os.Getenv("NEO4J_PASSWORD")
	if err := db.InitNeo4jDriver(uri, "neo4j", password); err != nil {
		t.Fatal(err)
	}
	defer db.CloseNeo4j()
	if err := db.EnsureGraphSchema(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.EnsureIndexes(ctx, client); err != nil {
		t.Fatal(err)
	}
	graph, err := neo4j.NewDriverWithContext(uri, neo4j.BasicAuth("neo4j", password, ""))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = graph.Close(ctx) }()

	const source = "https://pipeline.invalid/positive"
	var doc struct {
		ID         primitive.ObjectID `bson:"_id"`
		Source     string             `bson:"source"`
		SourceType string             `bson:"source_type"`
		ScrapedAt  time.Time          `bson:"scraped_at"`
		Relevance  struct {
			Relevant bool   `bson:"relevant"`
			Model    string `bson:"model_version"`
			Policy   string `bson:"policy_version"`
		} `bson:"relevance"`
		Entities entities.Entities `bson:"entities"`
	}
	if err := coll.FindOne(ctx, bson.M{"url": source}).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if doc.Source != source || doc.SourceType != "smoke_fixture" || doc.ScrapedAt.IsZero() ||
		!doc.Relevance.Relevant || doc.Relevance.Model != "pipeline-smoke-fixture-v1" || doc.Relevance.Policy == "" {
		t.Fatal("ingestion provenance or classification verdict missing")
	}
	if n, err := coll.CountDocuments(ctx, bson.M{"url": "https://pipeline.invalid/negative",
		"relevance.relevant": false, "relevance.model_version": doc.Relevance.Model}); err != nil || n != 1 {
		t.Fatalf("negative verdict missing: count=%d err=%v", n, err)
	}

	var calls atomic.Int32
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var req struct{ Messages []map[string]string }
		if r.Method != http.MethodPost || r.URL.Path != "/api/chat" ||
			json.NewDecoder(r.Body).Decode(&req) != nil || len(req.Messages) != 2 ||
			!strings.Contains(req.Messages[1]["content"], "192.0.2.42") {
			t.Error("unexpected extraction request (a non-relevant article may have arrived)")
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": map[string]string{"content": `{"iocs":{"ips":["192.0.2.42","invalid-ip"]}}`},
		})
	}))
	defer ollama.Close()
	primary := extractor.NewOllamaExtractor(ollama.URL, "fixture")
	extractPending(ctx, client, primary, nil)

	if err := coll.FindOne(ctx, bson.M{"_id": doc.ID}).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Entities.IOCs.IPs) != 1 || doc.Entities.IOCs.IPs[0] != "192.0.2.42" {
		t.Fatalf("validated IOC missing: %v", doc.Entities.IOCs.IPs)
	}
	projectPending(ctx, client, db.ReplaceEntityGraph)
	if n, err := coll.CountDocuments(ctx, bson.M{"_id": doc.ID, "graph_projected_at": bson.M{"$type": "date"}}); err != nil || n != 1 {
		t.Fatalf("first projection did not complete: count=%d err=%v", n, err)
	}
	// Simulate a lost commit acknowledgement: replay the same Neo4j write.
	if _, err := coll.UpdateByID(ctx, doc.ID, bson.M{"$unset": bson.M{"graph_projected_at": ""}}); err != nil {
		t.Fatal(err)
	}
	projectPending(ctx, client, db.ReplaceEntityGraph)
	extractPending(ctx, client, primary, nil)
	if calls.Load() != 1 {
		t.Fatalf("unexpected extraction calls: %d", calls.Load())
	}
	result, err := neo4j.ExecuteQuery(ctx, graph, `
		MATCH (ip:IP)-[r:MENTIONED_IN]->(a:Article)
		RETURN count(r) AS edges, collect(ip.value) AS values, collect(a.id) AS articles`,
		nil, neo4j.EagerResultTransformer)
	if err != nil {
		t.Fatal(err)
	}
	row := result.Records[0].AsMap()
	if row["edges"] != int64(1) || fmt.Sprint(row["values"]) != "[192.0.2.42]" ||
		fmt.Sprint(row["articles"]) != "["+doc.ID.Hex()+"]" {
		t.Fatalf("incorrect graph provenance or duplicate after replay: %v", row)
	}

	// Rétractation : un second article partage l'IOC puis perd son éligibilité.
	// Son noeud Article doit disparaître, l'IOC partagé doit rester rattaché au
	// premier article. Le document est supprimé ensuite pour ne pas perturber
	// les étapes suivantes du smoke.
	sibling := primitive.NewObjectID()
	if _, err := coll.InsertOne(ctx, bson.M{"_id": sibling, "url": "https://pipeline.invalid/retracted",
		"content": "Sibling article mentioning 192.0.2.42.", "relevance": bson.M{"relevant": true}}); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = coll.DeleteOne(context.Background(), bson.M{"_id": sibling}) }()
	extractPending(ctx, client, primary, nil)
	projectPending(ctx, client, db.ReplaceEntityGraph)
	if _, err := coll.UpdateByID(ctx, sibling, bson.M{"$set": bson.M{"relevance.relevant": false}}); err != nil {
		t.Fatal(err)
	}
	retractStale(ctx, client, db.RetractEntityGraph)
	if n, err := coll.CountDocuments(ctx, bson.M{"_id": sibling, "graph_projected_at": bson.M{"$exists": true}}); err != nil || n != 0 {
		t.Fatalf("retraction not acknowledged in Mongo: count=%d err=%v", n, err)
	}
	result, err = neo4j.ExecuteQuery(ctx, graph, `
		OPTIONAL MATCH (gone:Article {id: $sibling})
		WITH count(gone) AS gone
		MATCH (ip:IP {value: '192.0.2.42'})-[:MENTIONED_IN]->(a:Article)
		RETURN gone, collect(a.id) AS articles`,
		map[string]any{"sibling": sibling.Hex()}, neo4j.EagerResultTransformer)
	if err != nil {
		t.Fatal(err)
	}
	row = result.Records[0].AsMap()
	if row["gone"] != int64(0) || fmt.Sprint(row["articles"]) != "["+doc.ID.Hex()+"]" {
		t.Fatalf("retraction removed too much or too little: %v", row)
	}
}
