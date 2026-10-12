package scraper_tests

import (
	"context"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/db"
	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/sites"
)

func TestSaveScrapedPage_UpsertsIntoMongo(t *testing.T) {
	if os.Getenv("MONGO_HOST") == "" {
		t.Skip("MONGO_HOST non défini — lancer `docker compose up -d mongodb` et exporter les vars MONGO_*")
	}

	page := sites.ScrapedPage{
		URL:       "http://test-page.onion",
		Source:    "http://test-page.onion",
		ScrapedAt: time.Now().UTC(),
		Content:   "contenu de test",
	}

	n, err := sites.SaveScrapedPage(page)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 upserted doc, got %d", n)
	}

	client, err := db.ConnectMongo()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer client.Disconnect(context.Background())

	var got bson.M
	err = client.Database("BlueWhiteThreat").Collection("article").
		FindOne(context.Background(), bson.M{"url": "http://test-page.onion"}).Decode(&got)
	if err != nil {
		t.Fatalf("document not found: %v", err)
	}
	if got["content"] != "contenu de test" {
		t.Errorf("got content=%v", got["content"])
	}

	client.Database("BlueWhiteThreat").Collection("article").
		DeleteOne(context.Background(), bson.M{"url": "http://test-page.onion"})
}
