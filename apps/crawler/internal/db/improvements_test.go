package db

import (
	"fmt"
	"github.com/redis/go-redis/v9"
	"testing"
	"time"
)

func TestMetadataFencingAndRenewal(t *testing.T) {
	setupTestRedisClient(t)
	u := "http://fence.onion/page"
	if _, err := AddURLToQueue(u); err != nil {
		t.Fatal(err)
	}
	_, token, err := ClaimURL()
	if err != nil {
		t.Fatal(err)
	}
	if err := RenewClaim(u, token); err != nil {
		t.Fatal(err)
	}
	if err := SaveClaimMetadata(u, "wrong", "bad", "", "bad"); err == nil {
		t.Fatal("stale writer accepted")
	}
	if err := SaveClaimMetadata(u, token, "good", "", "good"); err != nil {
		t.Fatal(err)
	}
	RedisClient.ZAdd(ctx, "crawler:inflight", redis.Z{Score: float64(time.Now().Unix() - 1), Member: u})
	if err := RenewClaim(u, token); err == nil {
		t.Fatal("expired renewal accepted")
	}
	if err := SaveClaimMetadata(u, token, "bad", "", "bad"); err == nil {
		t.Fatal("expired writer accepted")
	}
	metadata, err := FetchMetadata(u)
	if err != nil || metadata["hash"] != "good" {
		t.Fatalf("%v %v", metadata, err)
	}
}

func TestSchedulerChoosesLeastRecentlyServedHost(t *testing.T) {
	setupTestRedisClient(t)
	AddURLToQueue("http://a.onion/1")
	AddURLToQueue("http://b.onion/1")
	RedisClient.ZAdd(ctx, "crawler:hosts", redis.Z{Score: 100, Member: "a.onion"}, redis.Z{Score: 50, Member: "b.onion"})
	u, _, err := ClaimURL()
	if err != nil || u != "http://b.onion/1" {
		t.Fatalf("%s %v", u, err)
	}
}

func TestDiscoveryLimits(t *testing.T) {
	setupTestRedisClient(t)
	for _, raw := range []string{"http://bad.example/path", "http://a.onion/a/a/a/a/a", "http://a.onion/?PHPSESSID=abc"} {
		reason, err := AddURLToQueue(raw)
		if err != nil || reason != "trap" {
			t.Fatalf("%s: %s %v", raw, reason, err)
		}
	}
	RedisClient.Set(ctx, fmt.Sprintf("crawler:discovery_daily:budget.onion:%d", time.Now().Unix()/86400), 10000, time.Hour)
	reason, err := AddURLToQueue("http://budget.onion/new")
	if err != nil || reason != "budget" {
		t.Fatalf("%s %v", reason, err)
	}
}

func TestRenderedHandoffFiltersAndAcknowledges(t *testing.T) {
	setupTestRedisClient(t)
	RedisClient.RPush(ctx, "crawler:rendered_links", `{"Source":"http://a.onion","Links":["http://b.onion/page","http://example.com"]}`)
	if err := drainRenderedLinks(func(string, []string) error { return nil }); err != nil {
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
		AddURLToQueue(u)
		claimed, token, err := ClaimURL()
		if err != nil {
			t.Fatal(err)
		}
		if err := SaveClaimMetadata(claimed, token, "", "", "shared-hash"); err != nil {
			t.Fatal(err)
		}
	}
	a, _ := FetchMetadata("http://a.onion")
	b, _ := FetchMetadata("http://b.onion")
	if a["content_canonical"] == "" || a["content_canonical"] != b["content_canonical"] {
		t.Fatalf("%v %v", a, b)
	}
	if err := RecordHostFetch("http://a.onion/path", 429, time.Second, true); err != nil {
		t.Fatal(err)
	}
	if n, _ := RedisClient.HGet(ctx, "crawler:host_stats:a.onion", "errors").Int(); n != 1 {
		t.Fatal(n)
	}
}

func TestRenderedGraphFailureKeepsMessage(t *testing.T) {
	setupTestRedisClient(t)
	RedisClient.RPush(ctx, "crawler:rendered_links", `{"Source":"http://a.onion","Links":["http://b.onion/page"]}`)
	if err := drainRenderedLinks(func(source string, links []string) error {
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
	reason, err := AddURLToQueue("http://a.onion/later")
	if err != nil || reason != "budget" {
		t.Fatalf("%s %v", reason, err)
	}
	if _, err := RedisClient.ZScore(ctx, "crawler:scheduled", "http://a.onion/later").Result(); err != nil {
		t.Fatal("discovery lost")
	}
	RedisClient.Del(ctx, key)
	reason, err = AddURLToQueue("http://a.onion/new")
	if err != nil || reason != "added" {
		t.Fatalf("%s %v", reason, err)
	}
}

func TestCanonicalVersionInvalidation(t *testing.T) {
	setupTestRedisClient(t)
	AddURLToQueue("http://a.onion")
	a, token, _ := ClaimURL()
	if err := SaveClaimMetadata(a, token, "", "", "old"); err != nil {
		t.Fatal(err)
	}
	if err := SaveClaimMetadata(a, token, "", "", "new"); err != nil {
		t.Fatal(err)
	}
	AddURLToQueue("http://b.onion")
	b, token, _ := ClaimURL()
	if err := SaveClaimMetadata(b, token, "", "", "old"); err != nil {
		t.Fatal(err)
	}
	metadata, _ := FetchMetadata(b)
	if metadata["content_canonical"] != b {
		t.Fatalf("stale canonical %v", metadata)
	}
}
