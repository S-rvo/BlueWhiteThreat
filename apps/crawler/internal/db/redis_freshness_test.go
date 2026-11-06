package db

import (
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func setupTestRedisClient(t *testing.T) {
	if RedisClient != nil {
		return
	}
	host := os.Getenv("REDIS_HOST")
	if host == "" {
		host = "localhost"
	}
	RedisClient = redis.NewClient(&redis.Options{Addr: host + ":6379"})
	if err := RedisClient.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis non disponible (%v) — lancer `docker compose up -d redis`", err)
	}
}

func cleanFreshnessKeys() {
	RedisClient.Del(ctx, "urls_crawled", "urls_crawled_at", "urls_to_crawl", "urls_to_crawl_priority", "crawler:stats")
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

	// The stale URL's timestamp must be bumped so a second sweep doesn't
	// immediately pick it again before it's actually revisited.
	score, err := RedisClient.ZScore(ctx, "urls_crawled_at", stale).Result()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if int64(score) < now.Unix() {
		t.Errorf("stale URL timestamp not bumped: %v", score)
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
