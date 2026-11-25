package sites

import (
	"context"
	"errors"
	"testing"
)

func TestScrapeOne_MalformedURLIsPermanent(t *testing.T) {
	err := scrapeOne(context.Background(), "ht!tp://[invalid", nil)
	if err == nil {
		t.Fatal("expected error for malformed URL")
	}
	if !errors.Is(err, ErrPermanentScrapeFailure) {
		t.Fatalf("expected ErrPermanentScrapeFailure, got: %v", err)
	}
}

func TestScraperWorkersPerProxy(t *testing.T) {
	cases := map[string]int{
		"":     defaultWorkersPerProxy,
		"0":    defaultWorkersPerProxy,
		"-1":   defaultWorkersPerProxy,
		"abc":  defaultWorkersPerProxy,
		"3":    3,
		"6":    6,
		"1000": maxWorkersPerProxy,
	}
	for raw, want := range cases {
		t.Setenv("SCRAPER_WORKERS_PER_PROXY", raw)
		if got := scraperWorkersPerProxy(); got != want {
			t.Errorf("SCRAPER_WORKERS_PER_PROXY=%q: got %d, want %d", raw, got, want)
		}
	}
}
