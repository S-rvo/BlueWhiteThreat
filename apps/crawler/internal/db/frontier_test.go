package db

import (
	"fmt"
	"github.com/redis/go-redis/v9"
	"testing"
	"time"
)

func TestLeaseRecoveryAndFencing(t *testing.T) {
	setupTestRedisClient(t)
	u := "http://lease.onion/path"
	if _, err := AddURLToQueue(u); err != nil {
		t.Fatal(err)
	}
	got, token, err := ClaimURL()
	if err != nil || got != u {
		t.Fatalf("%q %v", got, err)
	}
	RedisClient.ZAdd(ctx, "crawler:inflight", redis.Z{Score: 0, Member: u})
	RedisClient.HDel(ctx, "crawler:host_ready", "lease.onion")
	got, next, err := ClaimURL()
	if err != nil || got != u {
		t.Fatalf("recovery %q %v", got, err)
	}
	if err := FinishClaim(u, token, true, time.Hour); err == nil {
		t.Fatal("old lease accepted")
	}
	if err := FinishClaim(u, next, true, time.Hour); err != nil {
		t.Fatal(err)
	}
	if n, _ := RedisClient.ZCard(ctx, "crawler:scheduled").Result(); n != 1 {
		t.Fatal(n)
	}
}

func TestHostBackoffAndFreshness(t *testing.T) {
	setupTestRedisClient(t)
	a, b := "http://same.onion/a", "http://same.onion/b"
	AddURLToQueue(a)
	AddURLToQueue(b)
	u, token, err := ClaimURL()
	if err != nil || u == "" {
		t.Fatal(err)
	}
	if other, _, _ := ClaimURL(); other != "" {
		t.Fatal("parallel request to same host")
	}
	if err := FinishClaim(u, token, false, time.Hour); err != nil {
		t.Fatal(err)
	}
	if other, _, _ := ClaimURL(); other != "" {
		t.Fatal("host backoff ignored")
	}
	if n, _ := RedisClient.ZCard(ctx, "urls_crawled_at").Result(); n != 0 {
		t.Fatal("failed request recorded as fresh")
	}
}
func TestCapacityDefersWithoutLosingURL(t *testing.T) {
	setupTestRedisClient(t)
	for i := 0; i < maxQueueSize; i++ {
		RedisClient.SAdd(ctx, "urls_to_crawl", fmt.Sprintf("http://capacity%d.onion", i))
	}
	u := "http://deferred.onion/path"
	reason, err := AddURLToQueue(u)
	if err != nil || reason != QueueReasonCapacity {
		t.Fatalf("%s %v", reason, err)
	}
	if n, _ := GetQueueSize(); n != maxQueueSize {
		t.Fatal(n)
	}
	if _, err := RedisClient.ZScore(ctx, "crawler:scheduled", u).Result(); err != nil {
		t.Fatal("discovery lost", err)
	}
}
