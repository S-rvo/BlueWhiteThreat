package crawler_tests

import (
	"github.com/S-rvo/BlueWhiteThreat/apps/crawler/internal/crawler"
	"testing"
)

func TestInvalidURLRejected(t *testing.T) {
	_, _, _, err := crawler.Crawler("http://example.com/looks.onion", 0)
	if err == nil {
		t.Fatal("non-onion host accepted")
	}
}
