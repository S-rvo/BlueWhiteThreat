package crawler_test

import (
	"testing"

	"github.com/S-rvo/BlueWhiteThreat/internal/crawler"
)

func TestCrawlerInitialization(t *testing.T) {
	// Test unitaire basique : juste vérifier que la fonction Crawler existe et retourne bien 4 valeurs
	startURL := "http://example.onion"
	depth := 0

	_, _, _, err := crawler.Crawler(startURL, depth)
	if err != nil {
		t.Logf("Expected error (no real proxy): %v", err)
	}
}
