package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/db"
	"go.mongodb.org/mongo-driver/bson"
)

// Opt-in: use only the dedicated test database populated by the Telegram CLI.
// Logs contain counts and versions, never collected message contents.
func TestTelegramMongoIntegration(t *testing.T) {
	if os.Getenv("BWT_TELEGRAM_INTEGRATION") != "1" {
		t.Skip("set BWT_TELEGRAM_INTEGRATION=1 against the isolated Telegram test database")
	}
	if os.Getenv("MONGO_DB") != "BlueWhiteThreatTelegramTest" || os.Getenv("MONGO_PORT") != "27018" || os.Getenv("MONGO_HOST") != "127.0.0.1" {
		t.Fatal("integration test requires isolated BlueWhiteThreatTelegramTest at 127.0.0.1:27018")
	}
	classifier, err := loadClassifier(filepath.Join("..", "..", "model.json"), "")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := db.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Disconnect(ctx)
	collection := client.Database(os.Getenv("MONGO_DB")).Collection("article")
	count, err := collection.CountDocuments(ctx, bson.M{"source": "telegram"})
	if err != nil || count == 0 {
		t.Fatalf("expected collected Telegram articles: count=%d err=%v", count, err)
	}
	pending, err := db.FetchNeedingClassification(ctx, client, classifier.Version())
	if err != nil {
		t.Fatal(err)
	}
	for _, article := range pending {
		if _, ok := article.ID.(string); !ok {
			t.Fatalf("expected Telegram string ID, got %T", article.ID)
		}
	}
	runOnce(classifier)
	remaining, err := db.FetchNeedingClassification(ctx, client, classifier.Version())
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatalf("%d articles remain unclassified", len(remaining))
	}
	scored, err := collection.CountDocuments(ctx, bson.M{"source": "telegram", "relevance.model_version": classifier.Version(), "relevance.policy_version": db.ClassificationPolicyVersion})
	if err != nil || scored != count {
		t.Fatalf("scored=%d expected=%d err=%v", scored, count, err)
	}
	relevant, err := collection.CountDocuments(ctx, bson.M{"source": "telegram", "relevance.relevant": true})
	if err != nil {
		t.Fatal(err)
	}
	runOnce(classifier) // The second pass should find nothing pending.
	after, err := collection.CountDocuments(ctx, bson.M{"source": "telegram"})
	if err != nil || after != count {
		t.Fatalf("article count changed: %d -> %d (%v)", count, after, err)
	}
	t.Logf("telegram_articles=%d classified=%d relevant=%d pending=0 model=%s", count, scored, relevant, classifier.Version())
}
