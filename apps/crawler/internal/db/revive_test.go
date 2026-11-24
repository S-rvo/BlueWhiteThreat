package db

import (
	"github.com/redis/go-redis/v9"
	"testing"
	"time"
)

func TestReviveDeadBoundedAndNotFresh(t *testing.T) {
	setupTestRedisClient(t)
	old := float64(time.Now().Add(-8 * 24 * time.Hour).Unix())
	for _, u := range []string{"http://old.onion/", "http://other.onion/"} {
		RedisClient.ZAdd(ctx, "crawler:dead", redis.Z{Score: old, Member: u})
		RedisClient.HSet(ctx, "crawler:attempts", u, 10)
	}
	RedisClient.ZAdd(ctx, "crawler:dead", redis.Z{Score: float64(time.Now().Unix()), Member: "http://recent.onion/"})
	n, err := ReviveDeadURLs(ctx, 7*24*time.Hour, 1)
	if err != nil || n != 1 {
		t.Fatalf("%d %v", n, err)
	}
	if RedisClient.ZCard(ctx, "crawler:dead").Val() != 2 || RedisClient.ZCard(ctx, "crawler:scheduled").Val() != 1 {
		t.Fatal("bad frontier sizes")
	}
	if RedisClient.ZCard(ctx, "urls_crawled_at").Val() != 0 {
		t.Fatal("revival marked fresh")
	}
	u := RedisClient.ZRange(ctx, "crawler:scheduled", 0, -1).Val()[0]
	if RedisClient.HExists(ctx, "crawler:attempts", u).Val() {
		t.Fatal("attempt count retained")
	}
	if _, err := ReviveDeadURLs(ctx, time.Hour, 100); err == nil {
		t.Fatal("short cooldown accepted")
	}
}
