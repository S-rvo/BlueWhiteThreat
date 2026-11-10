package db

import (
	"github.com/alicebob/miniredis/v2"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

var testRedis *miniredis.Miniredis

func setupTestRedisClient(t *testing.T) {
	// Idempotence keyed on our OWN miniredis handle, never on RedisClient:
	// if an earlier test (or integration flow) left RedisClient pointing at a
	// real Redis, we must still stand up an isolated miniredis and rebind to
	// it — otherwise cleanFreshnessKeys would Del against production. This is
	// not hypothetical: a stale RedisClient here wiped the live frontier once.
	if testRedis != nil {
		return
	}
	server := miniredis.RunT(t)
	testRedis = server
	RedisClient = redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { RedisClient.Close(); RedisClient = nil; testRedis = nil })
}

func cleanFreshnessKeys() {
	// Last line of defense: these Del targets are the durable frontier itself.
	// Refuse to run unless RedisClient is provably our isolated miniredis, so a
	// misconfigured client can never delete production keys.
	if testRedis == nil || RedisClient == nil || RedisClient.Options().Addr != testRedis.Addr() {
		panic("cleanFreshnessKeys: refusing to Del against a non-miniredis Redis (call setupTestRedisClient first)")
	}
	RedisClient.Del(ctx, "urls_crawled", "urls_crawled_at", "urls_to_crawl", "urls_to_crawl_priority", "crawler:stats", "crawler:scheduled", "crawler:inflight", "crawler:tokens", "crawler:host_ready", "crawler:attempts")
}

func TestMarkURLAsCrawled_RecordsTimestamp(t *testing.T) {
	setupTestRedisClient(t)
	cleanFreshnessKeys()
	defer cleanFreshnessKeys()

	url := "http://fresh-test-a.onion"
	before := time.Now().Unix()
	if err := MarkURLAsCrawled(url); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	score, err := RedisClient.ZScore(ctx, "urls_crawled_at", url).Result()
	if err != nil {
		t.Fatalf("expected timestamp for %s: %v", url, err)
	}
	if int64(score) < before {
		t.Errorf("timestamp %v predates call start %v", score, before)
	}
}

func TestRequeueStaleURLs_RequeuesOnlyEntriesOlderThanMaxAge(t *testing.T) {
	setupTestRedisClient(t)
	cleanFreshnessKeys()
	defer cleanFreshnessKeys()

	stale := "http://fresh-test-stale.onion"
	fresh := "http://fresh-test-fresh.onion"
	now := time.Now()

	RedisClient.SAdd(ctx, "urls_crawled", stale, fresh)
	RedisClient.ZAdd(ctx, "urls_crawled_at", redis.Z{Score: float64(now.Add(-10 * 24 * time.Hour).Unix()), Member: stale})
	RedisClient.ZAdd(ctx, "urls_crawled_at", redis.Z{Score: float64(now.Unix()), Member: fresh})

	requeued, err := RequeueStaleURLs(7*24*time.Hour, 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if requeued != 1 {
		t.Fatalf("requeued = %d, want 1", requeued)
	}

	isQueued, _ := RedisClient.SIsMember(ctx, "urls_to_crawl", stale).Result()
	if !isQueued {
		t.Error("stale URL was not requeued")
	}
	freshQueued, _ := RedisClient.SIsMember(ctx, "urls_to_crawl", fresh).Result()
	if freshQueued {
		t.Error("fresh URL should not have been requeued")
	}

	// Scheduling must preserve the timestamp of the last successful fetch.
	score, err := RedisClient.ZScore(ctx, "urls_crawled_at", stale).Result()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if int64(score) != now.Add(-10*24*time.Hour).Unix() {
		t.Errorf("success timestamp changed before fetch: %v", score)
	}
}

func TestRequeueStaleURLs_RespectsLimit(t *testing.T) {
	setupTestRedisClient(t)
	cleanFreshnessKeys()
	defer cleanFreshnessKeys()

	old := float64(time.Now().Add(-10 * 24 * time.Hour).Unix())
	for i := 0; i < 5; i++ {
		url := "http://fresh-test-limit-" + string(rune('a'+i)) + ".onion"
		RedisClient.SAdd(ctx, "urls_crawled", url)
		RedisClient.ZAdd(ctx, "urls_crawled_at", redis.Z{Score: old, Member: url})
	}

	requeued, err := RequeueStaleURLs(7*24*time.Hour, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if requeued != 2 {
		t.Fatalf("requeued = %d, want 2 (bounded by limit)", requeued)
	}
}

func TestBackfillCrawledAt_SeedsNowAndNeverOverwrites(t *testing.T) {
	setupTestRedisClient(t)
	cleanFreshnessKeys()
	defer cleanFreshnessKeys()

	legacyURL := "http://fresh-test-legacy.onion"
	trackedURL := "http://fresh-test-tracked.onion"
	oldScore := float64(time.Now().Add(-30 * 24 * time.Hour).Unix())

	RedisClient.SAdd(ctx, "urls_crawled", legacyURL, trackedURL)
	RedisClient.ZAdd(ctx, "urls_crawled_at", redis.Z{Score: oldScore, Member: trackedURL})

	before := time.Now().Unix()
	if err := backfillCrawledAt(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	legacyScore, err := RedisClient.ZScore(ctx, "urls_crawled_at", legacyURL).Result()
	if err != nil {
		t.Fatalf("expected backfilled timestamp for legacy url: %v", err)
	}
	if int64(legacyScore) < before {
		t.Errorf("legacy url backfilled with stale timestamp %v, want >= %v (avoids mass-requeue burst)", legacyScore, before)
	}

	trackedScore, err := RedisClient.ZScore(ctx, "urls_crawled_at", trackedURL).Result()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if trackedScore != oldScore {
		t.Errorf("backfill overwrote existing timestamp: got %v, want %v (NX must preserve it)", trackedScore, oldScore)
	}
}
