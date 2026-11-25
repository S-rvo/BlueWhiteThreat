package sites

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/db"
	"go.mongodb.org/mongo-driver/bson"
)

// Run only against a dedicated Redis instance, never the crawler's live sets.
// The live classifier must run a cycle while this test waits for its output.
func TestPipelineThroughRunningNLP(t *testing.T) {
	if os.Getenv("BWT_PIPELINE_TEST") != "1" {
		t.Skip("opt-in integration test")
	}
	if os.Getenv("REDIS_PORT") == "" || os.Getenv("REDIS_PORT") == "6379" {
		t.Fatal("dedicated Redis port required")
	}
	ctx := context.Background()
	// Reject a nonempty instance before inserting or deleting any test keys.
	r := db.ConnectRedis()
	defer r.Close()
	if err := r.Ping(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	if n, err := r.DBSize(ctx).Result(); err != nil || n != 0 {
		t.Fatalf("test Redis must start empty: keys=%d err=%v", n, err)
	}
	modelBytes, err := os.ReadFile("/app/apps/classifier/model.json")
	if err != nil {
		t.Fatal(err)
	}
	var model struct {
		Version   string  `json:"version"`
		Threshold float64 `json:"threshold"`
	}
	if err := json.Unmarshal(modelBytes, &model); err != nil {
		t.Fatal(err)
	}
	m, err := db.ConnectMongo()
	if err != nil {
		t.Fatal(err)
	}
	defer m.Disconnect(ctx)
	coll := m.Database("BlueWhiteThreat").Collection("article")
	pages := map[string]string{
		"/positive":  "Ransomware threat intelligence report. The threat actor deployed malware using phishing emails and exploited CVE-2024-12345. The ransomware campaign exfiltrated stolen credentials and sensitive data. Indicators of compromise include command and control domain malicious.example and IP address 192.0.2.42. Security researchers detected the malware payload and recommend patching vulnerable servers.",
		"/negative":  "Today we prepare a delicious homemade apple pie. Mix flour butter sugar and cinnamon. Bake in the oven and serve with vanilla ice cream. Gardening tips for planting tomatoes and watering flowers in spring.",
		"/directory": "Onion links directory\nhttp://alpha.onion\nhttp://beta.onion\nhttp://gamma.onion\nhttp://delta.onion\nhttp://epsilon.onion",
		"/blocked":   "Synthetic safety fixture: CSAM",
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprintf(w, "<html><body><nav>NAVIGATION_NOISE</nav><article><p>%s</p></article><footer>FOOTER_NOISE</footer></body></html>", pages[req.URL.Path])
	}))
	defer server.Close()
	prefix := server.URL
	testURLs := make([]string, 0, len(pages))
	for path := range pages {
		testURLs = append(testURLs, prefix+path)
	}
	defer coll.DeleteMany(ctx, bson.M{"url": bson.M{"$in": testURLs}})
	defer r.Del(ctx, "urls_crawled", "urls_scraped")
	browser := newPooledBrowser("tor1:9050")
	defer browser.close()
	for path := range pages {
		u := prefix + path
		if err := r.SAdd(ctx, "urls_crawled", u).Err(); err != nil {
			t.Fatal(err)
		}
		err := scrapeOneGuarded(browser, u, nil)
		if path == "/blocked" {
			if err == nil {
				t.Fatal("safety fixture was persisted")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if err := db.MarkURLScraped(ctx, r, u); err != nil {
			t.Fatal(err)
		}
		// A repeated scrape must update the same document.
		if err := scrapeOneGuarded(browser, u, nil); err != nil {
			t.Fatal(err)
		}
		n, err := coll.CountDocuments(ctx, bson.M{"url": u})
		if err != nil || n != 1 {
			t.Fatalf("dedup %s count=%d err=%v", path, n, err)
		}
		var doc struct {
			Content   string    `bson:"content"`
			Source    string    `bson:"source"`
			ScrapedAt time.Time `bson:"scraped_at"`
		}
		if err := coll.FindOne(ctx, bson.M{"url": u}).Decode(&doc); err != nil {
			t.Fatal(err)
		}
		if doc.Content != pages[path] || doc.Source != u || doc.ScrapedAt.IsZero() || strings.Contains(doc.Content, "_NOISE") {
			t.Fatalf("content/provenance mismatch %s: %+v", path, doc)
		}
	}
	n, err := coll.CountDocuments(ctx, bson.M{"url": prefix + "/blocked"})
	if err != nil || n != 0 {
		t.Fatal("blocked content reached MongoDB")
	}
	pending, err := db.GetUnscrapedURLs(ctx, r)
	if err != nil || len(pending) != 1 || pending[0] != prefix+"/blocked" {
		t.Fatalf("Redis handoff mismatch: %v %v", pending, err)
	}
	t.Log("FIXTURES_READY: browser extraction, Redis handoff, MongoDB upsert, safety rejection verified; waiting for real NLP")
	deadline := time.Now().Add(100 * time.Second)
	for time.Now().Before(deadline) {
		n, err := coll.CountDocuments(ctx, bson.M{"url": bson.M{"$in": []string{prefix + "/positive", prefix + "/negative", prefix + "/directory"}}, "relevance.model_version": bson.M{"$exists": true}})
		if err != nil {
			t.Fatal(err)
		}
		if n == 3 {
			break
		}
		time.Sleep(time.Second)
	}
	var negativeScore float64
	for _, path := range []string{"/positive", "/negative", "/directory"} {
		var doc struct {
			Relevance struct {
				Relevant bool    `bson:"relevant"`
				Score    float64 `bson:"score"`
				Version  string  `bson:"model_version"`
				Policy   string  `bson:"policy_version"`
			} `bson:"relevance"`
		}
		if err := coll.FindOne(ctx, bson.M{"url": prefix + path}).Decode(&doc); err != nil {
			t.Fatal(err)
		}
		v := doc.Relevance
		if path == "/negative" {
			negativeScore = v.Score
		}
		if v.Version != model.Version || v.Policy == "" || math.IsNaN(v.Score) || math.IsInf(v.Score, 0) || v.Score < 0 || v.Score > 1 {
			t.Fatalf("invalid NLP output %s: %+v", path, v)
		}
		t.Logf("NLP %s relevant=%t score=%.6f model=%s policy=%s", path, v.Relevant, v.Score, v.Version, v.Policy)
		if path == "/directory" {
			if v.Relevant || v.Score != 0 {
				t.Error("directory exclusion was not applied")
			}
		} else if v.Relevant != (v.Score >= model.Threshold) {
			t.Errorf("stored verdict does not match model threshold for %s", path)
		}
		// Semantic accuracy is measured separately from plumbing correctness.
		// Keep every wrong label visible, without requiring 100% accuracy here.
		if v.Relevant != (path == "/positive") {
			t.Logf("MODEL_QUALITY_MISS path=%s expected_relevant=%t actual=%t score=%.6f threshold=%.6f", path, path == "/positive", v.Relevant, v.Score, model.Threshold)
		}
	}
	// Update a previously scored document, then require the SAME running model
	// to classify its new text rather than retaining the old version's score.
	updatedURL := prefix + "/positive"
	if _, err := SaveScrapedPage(ScrapedPage{URL: updatedURL, Source: updatedURL, Content: pages["/negative"], ScrapedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	var pendingVersion bson.M
	if err := coll.FindOne(ctx, bson.M{"url": updatedURL}).Decode(&pendingVersion); err != nil {
		t.Fatal(err)
	}
	if _, ok := pendingVersion["relevance"]; ok {
		t.Fatal("new content kept old relevance")
	}
	t.Log("UPDATED_FIXTURE_READY: waiting for the same NLP model to classify the edited page")
	deadline = time.Now().Add(100 * time.Second)
	for time.Now().Before(deadline) {
		var updated struct {
			Relevance *struct {
				Score   float64 `bson:"score"`
				Version string  `bson:"model_version"`
			} `bson:"relevance"`
		}
		if err := coll.FindOne(ctx, bson.M{"url": updatedURL}).Decode(&updated); err != nil {
			t.Fatal(err)
		}
		if updated.Relevance != nil {
			if updated.Relevance.Version != model.Version || math.Abs(updated.Relevance.Score-negativeScore) > 1e-12 {
				t.Fatalf("edited page was not rescored correctly: %+v", updated.Relevance)
			}
			t.Logf("UPDATED_PAGE_RECLASSIFIED: score=%.6f model=%s", updated.Relevance.Score, updated.Relevance.Version)
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatal("edited page was not classified before deadline")
}

func TestPipelineContentUpdateInvalidatesRelevance(t *testing.T) {
	if os.Getenv("BWT_PIPELINE_TEST") != "1" {
		t.Skip("opt-in integration test")
	}
	ctx := context.Background()
	m, err := db.ConnectMongo()
	if err != nil {
		t.Fatal(err)
	}
	defer m.Disconnect(ctx)
	coll := m.Database("BlueWhiteThreat").Collection("article")
	u := fmt.Sprintf("http://pipeline-test.invalid/update-%d", time.Now().UnixNano())
	defer coll.DeleteOne(ctx, bson.M{"url": u})
	if _, err := coll.InsertOne(ctx, bson.M{"url": u, "content": "old text", "relevance": bson.M{"relevant": true, "model_version": "test-version", "policy_version": "test-policy"}}); err != nil {
		t.Fatal(err)
	}
	// Identical content preserves the verdict even with a newer scrape date.
	if _, err := SaveScrapedPage(ScrapedPage{URL: u, Source: u, Content: "old text", ScrapedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	var unchanged bson.M
	if err := coll.FindOne(ctx, bson.M{"url": u}).Decode(&unchanged); err != nil {
		t.Fatal(err)
	}
	if _, ok := unchanged["relevance"]; !ok {
		t.Fatal("unchanged content lost its verdict")
	}
	if _, err := SaveScrapedPage(ScrapedPage{URL: u, Source: u, Content: "$literal-looking new article text", ScrapedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	var result bson.M
	if err := coll.FindOne(ctx, bson.M{"url": u}).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result["content"] != "$literal-looking new article text" {
		t.Fatal("content was not updated")
	}
	if _, exists := result["relevance"]; exists {
		t.Error("content changed but old NLP relevance remains: the same model/policy will not reclassify this version")
	}
}
