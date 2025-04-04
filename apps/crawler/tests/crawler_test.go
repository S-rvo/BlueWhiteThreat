package crawler_test

import (
	"testing"

	"github.com/S-rvo/BlueWhiteThreat/internal/crawler"
)

func TestCrawlerReturns200(t *testing.T) {
	startURL := "http://6nhmgdpnyoljh5uzr5kwlatx2u3diou4ldeommfxjz3wkhalzgjqxzqd.onion"
	depth := 1

	visited, links, statusCode, err := crawler.Crawler(startURL, depth)
	if err != nil {
		t.Fatalf("Crawler() returned error: %v", err)
	}

	if len(visited) == 0 {
		t.Error("Crawler() did not visit any URL")
	}

	if statusCode != 200 {
		t.Errorf("Expected status code 200, got %d", statusCode)
	}

	if len(links) == 0 {
		t.Log("Warning: No links were found on the page (this may be expected depending on the URL).")
	} else {
		t.Logf("Found %d links on the page.", len(links))
	}
}
