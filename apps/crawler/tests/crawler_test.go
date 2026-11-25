package crawler_tests

import (
	"context"
	"testing"

	"github.com/S-rvo/BlueWhiteThreat/apps/crawler/internal/crawler"
)

func TestInvalidURLRejected(t *testing.T) {
	_, err := crawler.FetchContext(context.Background(), "http://example.com/looks.onion", nil)
	if err == nil {
		t.Fatal("non-onion host accepted")
	}
}
