package db

import (
	"context"
	"fmt"
	"github.com/redis/go-redis/v9"
	"testing"
	"time"
)

func TestMetadataFencingAndRenewal(t *testing.T) {
	setupTestRedisClient(t)
	u := "http://fence.onion/page"
	if _, err := AddURLToQueue(ctx, u); err != nil {
		t.Fatal(err)
	}
	_, token, err := ClaimURL(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := RenewClaim(ctx, u, token); err != nil {
		t.Fatal(err)
	}
	if err := SaveClaimMetadata(ctx, u, "wrong", "bad", "", "bad"); err == nil {
		t.Fatal("stale writer accepted")
	}
	if err := SaveClaimMetadata(ctx, u, token, "good", "", "good"); err != nil {
		t.Fatal(err)
	}
	RedisClient.ZAdd(ctx, "crawler:inflight", redis.Z{Score: float64(time.Now().Unix() - 1), Member: u})
	if err := RenewClaim(ctx, u, token); err == nil {
		t.Fatal("expired renewal accepted")
	}
	if err := SaveClaimMetadata(ctx, u, token, "bad", "", "bad"); err == nil {
		t.Fatal("expired writer accepted")
	}
	metadata, err := FetchMetadata(ctx, u)
	if err != nil || metadata["hash"] != "good" {
		t.Fatalf("%v %v", metadata, err)
	}
}

func TestSchedulerChoosesLeastRecentlyServedHost(t *testing.T) {
	setupTestRedisClient(t)
	AddURLToQueue(ctx, "http://a.onion/1")
	AddURLToQueue(ctx, "http://b.onion/1")
	RedisClient.ZAdd(ctx, "crawler:hosts", redis.Z{Score: 100, Member: "a.onion"}, redis.Z{Score: 50, Member: "b.onion"})
	u, _, err := ClaimURL(ctx)
	if err != nil || u != "http://b.onion/1" {
		t.Fatalf("%s %v", u, err)
	}
}

func TestDiscoveryLimits(t *testing.T) {
	setupTestRedisClient(t)
	for _, raw := range []string{"http://bad.example/path", "http://a.onion/a/a/a/a/a", "http://a.onion/?PHPSESSID=abc"} {
		reason, err := AddURLToQueue(ctx, raw)
		if err != nil || reason != "trap" {
			t.Fatalf("%s: %s %v", raw, reason, err)
		}
	}
	RedisClient.Set(ctx, fmt.Sprintf("crawler:discovery_daily:budget.onion:%d", time.Now().Unix()/86400), 10000, time.Hour)
	reason, err := AddURLToQueue(ctx, "http://budget.onion/new")
	if err != nil || reason != "budget" {
		t.Fatalf("%s %v", reason, err)
	}
}

func TestRenderedHandoffFiltersAndAcknowledges(t *testing.T) {
	setupTestRedisClient(t)
	RedisClient.RPush(ctx, "crawler:rendered_links", `{"Source":"http://a.onion","Links":["http://b.onion/page","http://example.com"]}`)
	if err := drainRenderedLinks(ctx, func(context.Context, string, []string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if n, _ := GetQueueSize(); n != 2 {
		t.Fatal(n)
	}
	if n, _ := RedisClient.LLen(ctx, "crawler:rendered_links").Result(); n != 0 {
		t.Fatal(n)
	}
}
func TestContentCanonicalAndHostMetrics(t *testing.T) {
	setupTestRedisClient(t)
	for _, u := range []string{"http://a.onion", "http://b.onion"} {
		AddURLToQueue(ctx, u)
		claimed, token, err := ClaimURL(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := SaveClaimMetadata(ctx, claimed, token, "", "", "shared-hash"); err != nil {
			t.Fatal(err)
		}
	}
	a, _ := FetchMetadata(ctx, "http://a.onion")
	b, _ := FetchMetadata(ctx, "http://b.onion")
	if a["content_canonical"] == "" || a["content_canonical"] != b["content_canonical"] {
		t.Fatalf("%v %v", a, b)
	}
	if err := RecordHostFetch(ctx, "http://a.onion/path", 429, time.Second, true); err != nil {
		t.Fatal(err)
	}
	if n, _ := RedisClient.HGet(ctx, "crawler:host_stats:a.onion", "errors").Int(); n != 1 {
		t.Fatal(n)
	}
}

func TestRenderedGraphFailureKeepsMessage(t *testing.T) {
	setupTestRedisClient(t)
	RedisClient.RPush(ctx, "crawler:rendered_links", `{"Source":"http://a.onion","Links":["http://b.onion/page"]}`)
	if err := drainRenderedLinks(ctx, func(_ context.Context, source string, links []string) error {
		if source != "http://a.onion" || len(links) != 1 {
			t.Fatalf("%s %v", source, links)
		}
		return fmt.Errorf("storage down")
	}); err == nil {
		t.Fatal("storage error swallowed")
	}
	if n, _ := RedisClient.ZCard(ctx, "crawler:rendered_retry").Result(); n != 1 {
		t.Fatal("message lost")
	}
}
func TestBudgetDeferredDurably(t *testing.T) {
	setupTestRedisClient(t)
	key := fmt.Sprintf("crawler:discovery_daily:a.onion:%d", time.Now().Unix()/86400)
	RedisClient.Set(ctx, key, 10000, time.Hour)
	reason, err := AddURLToQueue(ctx, "http://a.onion/later")
	if err != nil || reason != "budget" {
		t.Fatalf("%s %v", reason, err)
	}
	if _, err := RedisClient.ZScore(ctx, "crawler:scheduled", "http://a.onion/later").Result(); err != nil {
		t.Fatal("discovery lost")
	}
	RedisClient.Del(ctx, key)
	reason, err = AddURLToQueue(ctx, "http://a.onion/new")
	if err != nil || reason != "added" {
		t.Fatalf("%s %v", reason, err)
	}
}

func TestCanonicalVersionInvalidation(t *testing.T) {
	setupTestRedisClient(t)
	AddURLToQueue(ctx, "http://a.onion")
	a, token, _ := ClaimURL(ctx)
	if err := SaveClaimMetadata(ctx, a, token, "", "", "old"); err != nil {
		t.Fatal(err)
	}
	if err := SaveClaimMetadata(ctx, a, token, "", "", "new"); err != nil {
		t.Fatal(err)
	}
	AddURLToQueue(ctx, "http://b.onion")
	b, token, _ := ClaimURL(ctx)
	if err := SaveClaimMetadata(ctx, b, token, "", "", "old"); err != nil {
		t.Fatal(err)
	}
	metadata, _ := FetchMetadata(ctx, b)
	if metadata["content_canonical"] != b {
		t.Fatalf("stale canonical %v", metadata)
	}
}
