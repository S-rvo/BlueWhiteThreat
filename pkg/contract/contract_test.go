package contract

import (
	"strings"
	"testing"
)

// Valeurs figées : changer un nom ici casse les données déjà en base ou les
// files Redis en cours. Toute modification exige une migration explicite.
func TestArticleSchemaPinned(t *testing.T) {
	pinned := map[string]string{
		Database: "BlueWhiteThreat", ArticleCollection: "article",
		FieldID: "_id", FieldURL: "url", FieldSource: "source", FieldSourceType: "source_type",
		FieldScrapedAt: "scraped_at", FieldContent: "content", FieldRelevance: "relevance",
		FieldEntities: "entities", FieldExtractionFailures: "extraction_failures",
		FieldExtractionRetryAt: "extraction_retry_at", FieldExtractionLastError: "extraction_last_error",
		FieldGraphProjectionQueuedAt: "graph_projection_queued_at", FieldGraphProjectedAt: "graph_projected_at",
		PathRelevanceRelevant: "relevance.relevant", PathRelevanceNeedsReview: "relevance.needs_review",
		PathRelevanceModelVersion: "relevance.model_version", PathRelevancePolicyVersion: "relevance.policy_version",
	}
	for got, want := range pinned {
		if got != want {
			t.Errorf("contrat article: %q, attendu %q", got, want)
		}
	}
	if len(ArticleFields()) != 13 || len(RelevanceFields()) != 7 {
		t.Fatalf("ArticleFields=%d RelevanceFields=%d", len(ArticleFields()), len(RelevanceFields()))
	}
}

func TestContentDerivedFieldsKeepGraphMarker(t *testing.T) {
	all := map[string]bool{}
	for _, f := range ArticleFields() {
		all[f] = true
	}
	for _, f := range ContentDerivedFields() {
		if !all[f] {
			t.Errorf("%q dérivé mais hors ArticleFields", f)
		}
		if f == FieldGraphProjectedAt || f == FieldContent || f == FieldURL {
			t.Errorf("%q ne doit pas être effacé au changement de contenu", f)
		}
	}
}

func TestSharedRedisKeysPinned(t *testing.T) {
	want := "urls_to_crawl urls_queued urls_crawled urls_crawled_at crawler:scheduled crawler:inflight " +
		"crawler:stats crawler:gone crawler:blocked crawler:hosts crawler:host_ready crawler:scheduled_hosts " +
		"crawler:scheduled_host: crawler:rendered_links crawler:rendered_inflight crawler:rendered_retry crawler:render_host_owner " +
		"scraper:pending scraper:inflight urls_scraped"
	if got := strings.Join(SharedRedisKeys(), " "); got != want {
		t.Fatalf("clés Redis partagées:\n got %s\nwant %s", got, want)
	}
}

func TestURLPriority(t *testing.T) {
	cases := map[string]int{
		"http://example.onion/":                         0,
		"http://example.onion/database":                 0,
		"http://example.onion/database-breach":          1,
		"http://example.onion/corporate-database-leak":  1,
		"http://example.onion/ACCESS-Broker":            2,
		"http://example.onion/initial-access":           2,
		"http://example.onion/ransomware/victims":       2,
		"http://example.onion/forum?topic=0day-exploit": 2,
	}
	for url, want := range cases {
		if got := URLPriority(url); got != want {
			t.Errorf("URLPriority(%q) = %d, attendu %d", url, got, want)
		}
	}
}

func TestPriorityStrideDominatesTimestamps(t *testing.T) {
	// Un horodatage Unix (secondes) reste sous le pas jusqu'en 2286.
	const year2200 = int64(7258118400)
	if PriorityStride <= year2200 {
		t.Fatalf("PriorityStride %d <= horodatage %d", PriorityStride, year2200)
	}
	if PendingPriorityBase("http://x.onion/ransomware") != 2*PriorityStride {
		t.Fatal("PendingPriorityBase mal aligné sur URLPriority")
	}
}
