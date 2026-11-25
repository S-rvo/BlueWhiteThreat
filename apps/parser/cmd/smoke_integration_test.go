//go:build integration

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/db"
	"github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/entities"
	"github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/extractor"
)

type countingSmokeExtractor struct {
	extractor.Extractor
	calls int
}

func (e *countingSmokeExtractor) Extract(ctx context.Context, text string) (entities.Entities, error) {
	e.calls++
	return e.Extractor.Extract(ctx, text)
}

func TestSmokeExtractAndReplay(t *testing.T) {
	if os.Getenv("BWT_DISPOSABLE_PIPELINE") != "1" {
		t.Skip("run tests/pipeline/smoke.py against disposable databases")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
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
	graph, err := neo4j.NewDriverWithContext(uri, neo4j.BasicAuth("neo4j", password, ""))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = graph.Close(ctx) }()

	const source = "https://pipeline.invalid/positive"
	ollamaModel := os.Getenv("BWT_SMOKE_OLLAMA_MODEL")
	sourceType, classifierVersion := "smoke_fixture", "pipeline-smoke-fixture-v1"
	if ollamaModel != "" {
		sourceType, classifierVersion = "darkweb_page", os.Getenv("BWT_SMOKE_CLASSIFIER_VERSION")
		if classifierVersion == "" {
			t.Fatal("expected classifier version required for real models")
		}
	}
	type smokeRelevance struct {
		Relevant    bool    `bson:"relevant"`
		Score       float64 `bson:"score"`
		NeedsReview bool    `bson:"needs_review"`
		Model       string  `bson:"model_version"`
		Policy      string  `bson:"policy_version"`
	}
	var doc struct {
		ID         primitive.ObjectID `bson:"_id"`
		Source     string             `bson:"source"`
		SourceType string             `bson:"source_type"`
		ScrapedAt  time.Time          `bson:"scraped_at"`
		Relevance  smokeRelevance     `bson:"relevance"`
		Entities   entities.Entities  `bson:"entities"`
	}
	if err := coll.FindOne(ctx, bson.M{"url": source}).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	var negative struct {
		Relevance smokeRelevance `bson:"relevance"`
	}
	if err := coll.FindOne(ctx, bson.M{"url": "https://pipeline.invalid/negative"}).Decode(&negative); err != nil {
		t.Fatal(err)
	}
	t.Logf("positive verdict: %+v; negative verdict: %v", doc.Relevance, negative.Relevance)
	if doc.Source != source || doc.SourceType != sourceType || doc.ScrapedAt.IsZero() ||
		doc.Relevance.Model != classifierVersion || doc.Relevance.Policy == "" {
		t.Fatal("ingestion provenance or positive verdict metadata missing")
	}
	if negative.Relevance.Relevant || negative.Relevance.Model != classifierVersion || negative.Relevance.Policy == "" {
		t.Fatal("negative verdict missing or incorrectly marked relevant")
	}
	pending, err := db.FetchUnparsedArticles(ctx, client, batchSize)
	if err != nil {
		t.Fatal(err)
	}
	if !doc.Relevance.Relevant || doc.Relevance.NeedsReview {
		t.Fatalf("classifier blocked the positive document (parser selected %d articles)", len(pending))
	}
	if len(pending) != 1 || pending[0].ID != doc.ID {
		t.Fatalf("parser did not select only the positive document (selected %d)", len(pending))
	}

	var ollamaURL string
	if ollamaModel == "" {
		ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		ollamaURL, ollamaModel = ollama.URL, "fixture"
	} else {
		port, err := strconv.ParseUint(os.Getenv("BWT_SMOKE_OLLAMA_PORT"), 10, 16)
		if err != nil || port == 0 {
			t.Fatal("a valid local Ollama port is required")
		}
		ollamaURL = fmt.Sprintf("http://127.0.0.1:%d", port)
	}
	primary := &countingSmokeExtractor{Extractor: extractor.NewOllamaExtractor(ollamaURL, ollamaModel)}
	extractPending(ctx, client, primary, nil)

	if err := coll.FindOne(ctx, bson.M{"_id": doc.ID}).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Entities.IOCs.IPs) != 1 || doc.Entities.IOCs.IPs[0] != "192.0.2.42" {
		t.Fatalf("validated IOC missing: %v", doc.Entities.IOCs.IPs)
	}
	projectPending(ctx, client, db.SaveEntityGraph)
	if n, err := coll.CountDocuments(ctx, bson.M{"_id": doc.ID, "graph_projected_at": bson.M{"$type": "date"}}); err != nil || n != 1 {
		t.Fatalf("first projection did not complete: count=%d err=%v", n, err)
	}
	// Simulate a lost commit acknowledgement: replay the same Neo4j write.
	if _, err := coll.UpdateByID(ctx, doc.ID, bson.M{"$unset": bson.M{"graph_projected_at": ""}}); err != nil {
		t.Fatal(err)
	}
	projectPending(ctx, client, db.SaveEntityGraph)
	extractPending(ctx, client, primary, nil)
	if primary.calls != 1 {
		t.Fatalf("unexpected extraction calls: %d", primary.calls)
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
		fmt.Sprint(row["articles"]) != "["+fmt.Sprint(doc.ID)+"]" {
		t.Fatalf("incorrect graph provenance or duplicate after replay: %v", row)
	}
	t.Log("validated IP linked to the source article in Neo4j; replay kept one edge and did not re-extract")
}
