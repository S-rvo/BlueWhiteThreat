package scraper_tests

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/db"
)

func cleanFreshnessKeys(ctx context.Context, client *redis.Client) {
	client.Del(ctx, "urls_crawled", "urls_scraped", "urls_scraped_at", "urls_scrape_failed_at")
}

func TestMarkURLScraped_RecordsTimestamp(t *testing.T) {
	ctx := context.Background()
	client := testRedisClient(t)
	cleanFreshnessKeys(ctx, client)
	defer cleanFreshnessKeys(ctx, client)

	url := "http://scraper-fresh-test-a.onion"
	before := time.Now().Unix()
	if err := db.MarkURLScraped(ctx, client, url); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	score, err := client.ZScore(ctx, "urls_scraped_at", url).Result()
	if err != nil {
		t.Fatalf("expected timestamp for %s: %v", url, err)
	}
	if int64(score) < before {
		t.Errorf("timestamp %v predates call start %v", score, before)
	}
}

func TestRequeueStaleScrapedURLs_ReexposesOnlyStaleEntries(t *testing.T) {
	ctx := context.Background()
	client := testRedisClient(t)
	cleanFreshnessKeys(ctx, client)
	defer cleanFreshnessKeys(ctx, client)

	stale := "http://scraper-fresh-test-stale.onion"
	fresh := "http://scraper-fresh-test-fresh.onion"
	now := time.Now()

	client.SAdd(ctx, "urls_crawled", stale, fresh)
	client.SAdd(ctx, "urls_scraped", stale, fresh)
	client.ZAdd(ctx, "urls_scraped_at", redis.Z{Score: float64(now.Add(-10 * 24 * time.Hour).Unix()), Member: stale})
	client.ZAdd(ctx, "urls_scraped_at", redis.Z{Score: float64(now.Unix()), Member: fresh})

	requeued, err := db.RequeueStaleScrapedURLs(ctx, client, 7*24*time.Hour, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if requeued != 1 {
		t.Fatalf("requeued = %d, want 1", requeued)
	}

	// Stale entry must disappear from urls_scraped/urls_scraped_at so it
	// naturally reappears via the existing SDiff(urls_crawled, urls_scraped)
	// pickup path, instead of a new parallel queue.
	stillScraped, _ := client.SIsMember(ctx, "urls_scraped", stale).Result()
	if stillScraped {
		t.Error("stale URL should have been removed from urls_scraped")
	}
	if _, err := client.ZScore(ctx, "urls_scraped_at", stale).Result(); err != redis.Nil {
		t.Error("stale URL should have been removed from urls_scraped_at")
	}

	unscraped, err := db.GetUnscrapedURLs(ctx, client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, u := range unscraped {
		if u == stale {
			found = true
		}
		if u == fresh {
			t.Error("fresh URL should not have been requeued")
		}
	}
	if !found {
		t.Error("stale URL did not reappear via GetUnscrapedURLs")
	}
}

func TestRequeueStaleScrapedURLs_RespectsLimit(t *testing.T) {
	ctx := context.Background()
	client := testRedisClient(t)
	cleanFreshnessKeys(ctx, client)
	defer cleanFreshnessKeys(ctx, client)

	old := float64(time.Now().Add(-10 * 24 * time.Hour).Unix())
	for i := 0; i < 5; i++ {
		url := "http://scraper-fresh-test-limit-" + string(rune('a'+i)) + ".onion"
		client.SAdd(ctx, "urls_scraped", url)
		client.ZAdd(ctx, "urls_scraped_at", redis.Z{Score: old, Member: url})
	}

	requeued, err := db.RequeueStaleScrapedURLs(ctx, client, 7*24*time.Hour, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if requeued != 2 {
		t.Fatalf("requeued = %d, want 2 (bounded by limit)", requeued)
	}
}

func TestMarkURLScrapeFailed_RecordsTimestamp(t *testing.T) {
	ctx := context.Background()
	client := testRedisClient(t)
	cleanFreshnessKeys(ctx, client)
	defer cleanFreshnessKeys(ctx, client)

	url := "http://scraper-fresh-test-failed-a.onion"
	before := time.Now().Unix()
	if err := db.MarkURLScrapeFailed(ctx, client, url); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	score, err := client.ZScore(ctx, "urls_scrape_failed_at", url).Result()
	if err != nil {
		t.Fatalf("expected timestamp for %s: %v", url, err)
	}
	if int64(score) < before {
		t.Errorf("timestamp %v predates call start %v", score, before)
	}
}

func TestRequeueFailedScrapedURLs_ReexposesOnlyStaleEntries(t *testing.T) {
	ctx := context.Background()
	client := testRedisClient(t)
	cleanFreshnessKeys(ctx, client)
	defer cleanFreshnessKeys(ctx, client)

	stale := "http://scraper-fresh-test-failed-stale.onion"
	fresh := "http://scraper-fresh-test-failed-fresh.onion"
	now := time.Now()

	client.SAdd(ctx, "urls_crawled", stale, fresh)
	client.SAdd(ctx, "urls_scraped", stale, fresh)
	client.ZAdd(ctx, "urls_scrape_failed_at", redis.Z{Score: float64(now.Add(-3 * time.Hour).Unix()), Member: stale})
	client.ZAdd(ctx, "urls_scrape_failed_at", redis.Z{Score: float64(now.Unix()), Member: fresh})

	requeued, err := db.RequeueFailedScrapedURLs(ctx, client, 2*time.Hour, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if requeued != 1 {
		t.Fatalf("requeued = %d, want 1", requeued)
	}

	stillScraped, _ := client.SIsMember(ctx, "urls_scraped", stale).Result()
	if stillScraped {
		t.Error("stale URL should have been removed from urls_scraped")
	}
	if _, err := client.ZScore(ctx, "urls_scrape_failed_at", stale).Result(); err != redis.Nil {
		t.Error("stale URL should have been removed from urls_scrape_failed_at")
	}

	unscraped, err := db.GetUnscrapedURLs(ctx, client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, u := range unscraped {
		if u == stale {
			found = true
		}
		if u == fresh {
			t.Error("fresh URL should not have been requeued")
		}
	}
	if !found {
		t.Error("stale URL did not reappear via GetUnscrapedURLs")
	}
}

func TestRequeueFailedScrapedURLs_RespectsLimit(t *testing.T) {
	ctx := context.Background()
	client := testRedisClient(t)
	cleanFreshnessKeys(ctx, client)
	defer cleanFreshnessKeys(ctx, client)

	old := float64(time.Now().Add(-3 * time.Hour).Unix())
	for i := 0; i < 5; i++ {
		url := "http://scraper-fresh-test-failed-limit-" + string(rune('a'+i)) + ".onion"
		client.SAdd(ctx, "urls_scraped", url)
		client.ZAdd(ctx, "urls_scrape_failed_at", redis.Z{Score: old, Member: url})
	}

	requeued, err := db.RequeueFailedScrapedURLs(ctx, client, 2*time.Hour, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if requeued != 2 {
		t.Fatalf("requeued = %d, want 2 (bounded by limit)", requeued)
	}
}
