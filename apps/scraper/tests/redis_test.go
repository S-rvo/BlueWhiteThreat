package scraper_tests

import (
	"context"
	"os"
	"testing"

	"github.com/redis/go-redis/v9"

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/db"
)

func testRedisClient(t *testing.T) *redis.Client {
	host := os.Getenv("REDIS_HOST")
	if host == "" {
		host = "localhost"
	}
	client := redis.NewClient(&redis.Options{Addr: host + ":6379"})
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Skipf("Redis non disponible (%v) — lancer `docker compose up -d redis`", err)
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
