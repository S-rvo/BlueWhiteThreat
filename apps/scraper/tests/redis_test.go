package scraper_tests

import (
	"context"
	"os"
	"testing"

	"github.com/redis/go-redis/v9"

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/db"
)

func testRedisClient(t *testing.T) *redis.Client {
	addr := os.Getenv("SCRAPER_TEST_REDIS")
	if addr == "" {
		t.Skip("requires explicit disposable SCRAPER_TEST_REDIS")
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { client.Close() })
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	// A dedicated instance must contain no unrelated crawler/application keys.
	keys, err := client.Keys(context.Background(), "*").Result()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		switch key {
		case "urls_crawled", "urls_scraped", "urls_scraped_at", "urls_scrape_failed_at":
		default:
			t.Fatalf("test Redis contains unrelated key %q", key)
		}
	}
	return client
}

func TestGetUnscrapedURLs_ReturnsOnlyNewURLs(t *testing.T) {
	ctx := context.Background()
	client := testRedisClient(t)
	client.Del(ctx, "urls_crawled", "urls_scraped")
	defer client.Del(ctx, "urls_crawled", "urls_scraped")

	client.SAdd(ctx, "urls_crawled", "http://a.onion", "http://b.onion", "http://c.onion")
	client.SAdd(ctx, "urls_scraped", "http://a.onion")

	got, err := db.GetUnscrapedURLs(ctx, client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := map[string]bool{"http://b.onion": true, "http://c.onion": true}
	if len(got) != 2 {
		t.Fatalf("got %d urls, want 2: %#v", len(got), got)
	}
	for _, u := range got {
		if !want[u] {
			t.Errorf("unexpected url in result: %s", u)
		}
	}
}

func TestMarkURLScraped_AddsToScrapedSet(t *testing.T) {
	ctx := context.Background()
	client := testRedisClient(t)
	client.Del(ctx, "urls_scraped")
	defer client.Del(ctx, "urls_scraped")

	if err := db.MarkURLScraped(ctx, client, "http://a.onion"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	isMember, err := client.SIsMember(ctx, "urls_scraped", "http://a.onion").Result()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isMember {
		t.Error("url not marked as scraped")
	}
}
