package db

import (
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
	RedisClient.HSet(ctx, "crawler:host_last_claim", "a.onion", 100, "b.onion", 50)
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
	RedisClient.HSet(ctx, "crawler:discovery_budget", "budget.onion", 10000)
	reason, err := AddURLToQueue("http://budget.onion/new")
	if err != nil || reason != "budget" {
		t.Fatalf("%s %v", reason, err)
	}
}
