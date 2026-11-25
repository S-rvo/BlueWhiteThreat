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
	if _, err := AddURLToQueue(ctx, u); err != nil {
		t.Fatal(err)
	}
	got, token, err := ClaimURL(ctx)
	if err != nil || got != u {
		t.Fatalf("%q %v", got, err)
	}
	RedisClient.ZAdd(ctx, "crawler:inflight", redis.Z{Score: 0, Member: u})
	RedisClient.HDel(ctx, "crawler:host_ready", "lease.onion")
	got, next, err := ClaimURL(ctx)
	if err != nil || got != u {
		t.Fatalf("recovery %q %v", got, err)
	}
	if err := FinishClaim(ctx, u, token, true, time.Hour); err == nil {
		t.Fatal("old lease accepted")
	}
	if err := FinishClaim(ctx, u, next, true, time.Hour); err != nil {
		t.Fatal(err)
	}
	if n, _ := RedisClient.ZCard(ctx, "crawler:scheduled").Result(); n != 1 {
		t.Fatal(n)
	}
}

func TestHostBackoffAndFreshness(t *testing.T) {
	setupTestRedisClient(t)
	a, b := "http://same.onion/a", "http://same.onion/b"
	AddURLToQueue(ctx, a)
	AddURLToQueue(ctx, b)
	u, token, err := ClaimURL(ctx)
	if err != nil || u == "" {
		t.Fatal(err)
	}
	if other, _, _ := ClaimURL(ctx); other != "" {
		t.Fatal("parallel request to same host")
	}
	if err := FinishClaim(ctx, u, token, false, time.Hour); err != nil {
		t.Fatal(err)
	}
	if other, _, _ := ClaimURL(ctx); other != "" {
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
	reason, err := AddURLToQueue(ctx, u)
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

func TestDeferredWorkBypassesFullReadySetAndHonorsHostLease(t *testing.T) {
	setupTestRedisClient(t)
	now := time.Now().Unix()
	busy := "busy.onion"
	RedisClient.HSet(ctx, "crawler:host_ready", busy, now+600)
	batch := make([]interface{}, 0, maxQueueSize)
	for i := 0; i < maxQueueSize; i++ {
		batch = append(batch, fmt.Sprintf("http://%s/item/%d", busy, i))
	}
	RedisClient.SAdd(ctx, "urls_to_crawl", batch...)
	RedisClient.ZAdd(ctx, "crawler:hosts", redis.Z{Score: float64(now + 600), Member: busy})
	RedisClient.ZAdd(ctx, "crawler:scheduled",
		redis.Z{Score: float64(now - 3600), Member: "http://busy.onion/old"},
		redis.Z{Score: float64(now - 600), Member: "http://available.onion/first"},
		redis.Z{Score: float64(now + 600), Member: "http://future.onion/future"})
	got, _, err := ClaimURL(ctx)
	if err != nil || got != "http://available.onion/first" {
		t.Fatalf("claim=%q err=%v", got, err)
	}
	if size := RedisClient.SCard(ctx, "urls_to_crawl").Val(); size != maxQueueSize {
		t.Fatalf("ready URLs lost: %d", size)
	}
	if got, _, err := ClaimURL(ctx); got != "" || err != nil {
		t.Fatalf("claimed a future/blocked URL %q, %v", got, err)
	}
}

func TestPerHostReadyBudgetKeepsOtherHostsInReady(t *testing.T) {
	setupTestRedisClient(t)
	for i := 0; i < maxReadyPerHost+1; i++ {
		got, err := AddURLToQueue(ctx, fmt.Sprintf("http://many.onion/p/%d", i))
		want := QueueReasonAdded
		if i == maxReadyPerHost {
			want = QueueReasonCapacity
		}
		if err != nil || got != want {
			t.Fatalf("admission %d = %q, %v; want %s", i, got, err, want)
		}
	}
	got, err := AddURLToQueue(ctx, "http://other.onion/p")
	if err != nil || got != QueueReasonAdded {
		t.Fatalf("different host lost ready slots: %s %v", got, err)
	}
}

func TestInfrastructureRetryAndGoneDoNotPolluteSiteFailures(t *testing.T) {
	setupTestRedisClient(t)
	u := "http://old.onion/removed"
	if _, err := AddURLToQueue(ctx, u); err != nil {
		t.Fatal(err)
	}
	_, token, err := ClaimURL(ctx)
	if err != nil || token == "" {
		t.Fatal(err)
	}
	RedisClient.HSet(ctx, "crawler:attempts", u, 9)
	if err := DeferClaim(ctx, u, token, time.Minute); err != nil {
		t.Fatal(err)
	}
	if got := RedisClient.HGet(ctx, "crawler:attempts", u).Val(); got != "9" {
		t.Fatalf("attempts after storage error = %q", got)
	}
	RedisClient.HDel(ctx, "crawler:host_ready", "old.onion")
	RedisClient.ZAdd(ctx, "crawler:scheduled", redis.Z{Score: 0, Member: u})
	_, token, err = ClaimURL(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := FinishClaim(ctx, u, token, true, 0, 410); err != nil {
		t.Fatal(err)
	}
	if got := RedisClient.ZCard(ctx, "crawler:gone").Val(); got != 1 {
		t.Fatalf("gone cardinality = %d", got)
	}
	if got := RedisClient.ZCard(ctx, "urls_crawled_at").Val(); got != 0 {
		t.Fatalf("gone counted as fresh: %d", got)
	}
	if reason, err := AddURLToQueue(ctx, u); err != nil || reason != QueueReasonDuplicate {
		t.Fatalf("gone URL got re-admitted: %s %v", reason, err)
	}
}

func TestScheduledHostIndexDiscardsStaleHeadsAndClaimsEligibleHost(t *testing.T) {
	setupTestRedisClient(t)
	now := time.Now().Unix()
	RedisClient.Set(ctx, "crawler:scheduled_index_next", now+600, 0)
	RedisClient.HSet(ctx, "crawler:host_ready", "busy.onion", now+300)
	const old = "http://busy.onion/old"
	const eligible = "http://available.onion/due"
	const stale = "http://available.onion/removed-by-older-worker"
	if err := RedisClient.Eval(ctx, scheduledHostsLua+`
scheduleURL(KEYS[1],ARGV[1],ARGV[4]-300)
scheduleURL(KEYS[1],ARGV[2],ARGV[4]-30)
scheduleURL(KEYS[1],ARGV[3],ARGV[4]-600)
redis.call('ZREM',KEYS[1],ARGV[3])
return 1`, []string{"crawler:scheduled"}, old, eligible, stale, now).Err(); err != nil {
		t.Fatal(err)
	}

	got, _, err := ClaimURL(ctx)
	if err != nil || got != eligible {
		t.Fatalf("claim=%q err=%v, want %q", got, err, eligible)
	}
	if RedisClient.ZCard(ctx, "crawler:scheduled").Val() != 1 ||
		RedisClient.ZCard(ctx, "crawler:scheduled_host:available.onion").Val() != 0 ||
		RedisClient.ZCard(ctx, "crawler:inflight").Val() != 1 {
		t.Fatal("index discarded canonical work or reused a stale head")
	}
}
