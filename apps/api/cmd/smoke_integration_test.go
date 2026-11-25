//go:build integration

package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestSmokeMongoContract(t *testing.T) {
	if os.Getenv("BWT_DISPOSABLE_PIPELINE") != "1" {
		t.Skip("run tests/pipeline/smoke.py against disposable databases")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(os.Getenv("MONGODB_URI")))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Disconnect(ctx) }()
	articles := client.Database("BlueWhiteThreat").Collection("article")
	// Read-only credentials must grant queries, not even a write matching no document.
	_, err = articles.UpdateOne(ctx, bson.M{"_id": "nonexistent-fixture"},
		bson.M{"$set": bson.M{"fixture": true}})
	var commandErr mongo.CommandError
	if !errors.As(err, &commandErr) || !commandErr.HasErrorCode(13) {
		t.Fatal("API credentials not verifiably read-only")
	}
	handler := newHandler(mongoStore{articles})
	request := func(path string, status int) string {
		t.Helper()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != status {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body)
		}
		return w.Body.String()
	}
	request("/readyz", http.StatusNoContent)
	body := request("/data?limit=2", http.StatusOK)
	if strings.Count(body, `"id":`) != 1 || !strings.Contains(body, `"source_type":"smoke_fixture"`) ||
		strings.Contains(body, "pipeline.invalid") || strings.Contains(body, "192.0.2.42") ||
		strings.Contains(body, `"content":`) {
		t.Fatalf("invalid minimal response: %s", body)
	}
	metrics := request("/metrics", http.StatusOK)
	for _, line := range []string{"bwt_articles 2", "bwt_classified_articles 2", "bwt_relevant_articles 1", "bwt_review_pending_articles 1",
		"bwt_extraction_pending_articles 0", "bwt_projection_pending_articles 0",
		"bwt_latest_scrape_timestamp_seconds 1795521600"} {
		if !strings.Contains(metrics, "\n"+line+"\n") {
			t.Fatalf("missing metric %q: %s", line, metrics)
		}
	}
}
