//go:build integration

package sites

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/db"
	"go.mongodb.org/mongo-driver/bson"
)

func TestSmokeIngest(t *testing.T) {
	if os.Getenv("BWT_DISPOSABLE_PIPELINE") != "1" {
		t.Skip("run tests/pipeline/smoke.py against disposable databases")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, err := db.ConnectMongo()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Disconnect(ctx) }()
	coll := client.Database("BlueWhiteThreat").Collection("article")
	if n, err := coll.CountDocuments(ctx, bson.M{}); err != nil || n != 0 {
		t.Fatalf("expected an empty smoke collection: count=%d err=%v", n, err)
	}
	pages := map[string]string{
		"positive": "Synthetic ransomware report. An example indicator is 192.0.2.42.",
		"negative": "The garden contains lavender and daisies.",
	}
	sourceType := "smoke_fixture"
	if os.Getenv("BWT_SMOKE_OLLAMA_MODEL") != "" {
		// Describe defensive observations only; the example IP is never contacted.
		pages["positive"] = "Incident response report: security analysts confirmed a ransomware infection at a fictional organization. " +
			"Endpoint telemetry showed malware execution and file encryption on several workstations. " +
			"Investigators correlated the affected hosts with repeated command-and-control traffic to IP address 192.0.2.42, " +
			"the sole network indicator of compromise in this synthetic report. " +
			"The security team isolated the affected hosts, preserved logs, and checked the remaining endpoints for signs of compromise."
		pages["negative"] = "The garden contains lavender and daisies. Each spring the neighbors plant vegetables, " +
			"water the flowers and share fresh tomatoes with friends. On Sunday they baked an apple pie for the picnic."
		sourceType = "darkweb_page"
	}
	for slug, expected := range pages {
		content, err := ExtractMainContent("<nav>navigation</nav><article><p>" + expected +
			"</p></article><footer>hors contenu</footer>")
		if err != nil || content != expected {
			t.Fatalf("HTML extraction %s: text=%q err=%v", slug, content, err)
		}
		u := "https://pipeline.invalid/" + slug
		page := ScrapedPage{
			URL: u, Source: u, SourceType: sourceType, Content: content,
			ScrapedAt: time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC),
		}
		for _, want := range []int{1, 0} {
			if n, err := SaveScrapedPage(page); err != nil || n != want {
				t.Fatalf("upsert %s: got %d, want %d, err=%v", slug, n, want, err)
			}
		}
	}
}
