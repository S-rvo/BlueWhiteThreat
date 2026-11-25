package db

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func queueRedis(t *testing.T) *redis.Client {
	t.Helper()
	server := miniredis.RunT(t)
	r := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func TestScrapeLeaseExpiresAndFencesLateWorker(t *testing.T) {
	ctx := context.Background()
	r := queueRedis(t)
	const u = "http://fixture.test/article"
	r.SAdd(ctx, "urls_crawled", u)
	if err := IndexScrapePending(ctx, r, 1); err != nil {
		t.Fatal(err)
	}
	first, err := ClaimScrape(ctx, r)
	if first.URL != u || err != nil {
		t.Fatalf("claim = %+v, %v", first, err)
	}
	if other, err := ClaimScrape(ctx, r); err != nil || other.URL != "" {
		t.Fatalf("duplicate claim = %+v, %v", other, err)
	}
	r.ZAdd(ctx, scrapeInflight, redis.Z{Member: u, Score: 0})
	recovered, err := ClaimScrape(ctx, r)
	if err != nil || recovered.URL != u || recovered.Token == first.Token {
		t.Fatalf("recovered = %+v, %v", recovered, err)
	}
	if err := FinishScrape(ctx, r, first, ScrapeSuccess); err == nil {
		t.Fatal("expired worker acknowledged newer claim")
	}
	if err := FinishScrape(ctx, r, recovered, ScrapeSuccess); err != nil {
		t.Fatal(err)
	}
	if !r.SIsMember(ctx, "urls_scraped", u).Val() || r.ZScore(ctx, "urls_scraped_at", u).Err() != nil {
		t.Fatal("expected one durable success")
	}
}

func TestScrapeBackoffDoesNotRetryEarlyOrReopenExhaustion(t *testing.T) {
	ctx := context.Background()
	r := queueRedis(t)
	const u = "http://fixture.test/retry"
	r.SAdd(ctx, "urls_crawled", u)
	if err := IndexScrapePending(ctx, r, 1); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 6; i++ {
		claim, err := ClaimScrape(ctx, r)
		if err != nil || claim.URL != u {
			t.Fatalf("claim %d: %+v %v", i, claim, err)
		}
		if err := FinishScrape(ctx, r, claim, ScrapeTransient); err != nil {
			t.Fatal(err)
		}
		if i == 6 {
			break
		}
		delay := time.Duration(2<<(i-1)) * time.Hour
		score := r.ZScore(ctx, "scraper:retry_due", u).Val()
		if score < float64(time.Now().Add(delay-time.Second).Unix()) {
			t.Fatalf("retry %d ignored backoff: %.0f", i, score)
		}
		// Simulate stale legacy failure timestamp; the due index must still win.
		r.ZAdd(ctx, "urls_scrape_failed_at", redis.Z{Member: u, Score: 1})
		if n, err := RequeueFailedScrapedURLs(ctx, r, 2*time.Hour, 100); err != nil || n != 0 {
			t.Fatalf("early retry: %d %v", n, err)
		}
		r.ZAdd(ctx, "scraper:retry_due", redis.Z{Member: u, Score: 1})
		if n, err := RequeueFailedScrapedURLs(ctx, r, 2*time.Hour, 100); err != nil || n != 1 {
			t.Fatalf("due retry: %d %v", n, err)
		}
	}
	if r.ZScore(ctx, "scraper:dead", u).Err() != nil || r.ZScore(ctx, "urls_scraped_at", u).Err() != redis.Nil {
		t.Fatal("exhausted URL not quarantined independently from successful scrapes")
	}
	if err := IndexScrapePending(ctx, r, 1); err != nil {
		t.Fatal(err)
	}
	if got, err := ClaimScrape(ctx, r); err != nil || got.URL != "" {
		t.Fatalf("quarantined reopened: %+v %v", got, err)
	}
}

func TestScrapeInfrastructureDoesNotConsumeRemoteAttempt(t *testing.T) {
	ctx := context.Background()
	r := queueRedis(t)
	const u = "http://fixture.test/storage"
	r.SAdd(ctx, "urls_crawled", u)
	if err := IndexScrapePending(ctx, r, 1); err != nil {
		t.Fatal(err)
	}
	claim, err := ClaimScrape(ctx, r)
	if err != nil || claim.URL != u {
		t.Fatalf("claim: %+v %v", claim, err)
	}
	if err := FinishScrape(ctx, r, claim, ScrapeInfrastructure); err != nil {
		t.Fatal(err)
	}
	if r.HExists(ctx, "scraper:attempts", u).Val() || r.ZScore(ctx, "scraper:retry_due", u).Err() != nil {
		t.Fatal("storage error counted as remote failure or not retried")
	}
}
