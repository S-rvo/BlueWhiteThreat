package scraper_tests

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/db"
)

func testRedisClient(t *testing.T) *redis.Client {
	// Create a private fixture, never infer a Redis address from the environment.
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

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
