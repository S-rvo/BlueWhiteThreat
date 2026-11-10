package db

import (
	"testing"
)

func TestDropClaimRemovesFromAllFrontierSetsAndDeadLetters(t *testing.T) {
	setupTestRedisClient(t)
	if _, err := AddURLToQueue("http://blocked.onion/x"); err != nil {
		t.Fatal(err)
	}
	url, token, err := ClaimURL()
	if err != nil || url == "" {
		t.Fatalf("claim: %q %v", url, err)
	}
	if err := DropClaim(url, token); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if n, _ := RedisClient.ZScore(ctx, "crawler:scheduled", url).Result(); n != 0 {
		t.Fatal("url still scheduled for recycling")
	}
	if n, _ := RedisClient.ZCard(ctx, "crawler:inflight").Result(); n != 0 {
		t.Fatal("url still inflight")
	}
	if ok, _ := RedisClient.SIsMember(ctx, "urls_to_crawl", url).Result(); ok {
		t.Fatal("url still in urls_to_crawl")
	}
	if ok, _ := RedisClient.HGet(ctx, "crawler:tokens", url).Result(); ok != "" {
		t.Fatal("token still set")
	}
	if n, _ := RedisClient.ZScore(ctx, "crawler:blocked", url).Result(); n == 0 {
		t.Fatal("url not in crawler:blocked")
	}

	// A subsequent claim never resurfaces the dropped URL.
	url2, _, err := ClaimURL()
	if err != nil {
		t.Fatal(err)
	}
	if url2 == url {
		t.Fatal("dropped url resurfaced on claim")
	}
}

func TestDropClaimWithBadTokenFails(t *testing.T) {
	setupTestRedisClient(t)
	if _, err := AddURLToQueue("http://blocked2.onion/x"); err != nil {
		t.Fatal(err)
	}
	url, _, err := ClaimURL()
	if err != nil || url == "" {
		t.Fatalf("claim: %q %v", url, err)
	}
	if err := DropClaim(url, "wrong-token"); err == nil {
		t.Fatal("expected lease error for wrong token")
	}
	if n, _ := RedisClient.ZCard(ctx, "crawler:inflight").Result(); n != 1 {
		t.Fatal("bad-token drop touched inflight")
	}
	if n, _ := RedisClient.ZScore(ctx, "crawler:blocked", url).Result(); n != 0 {
		t.Fatal("bad-token drop touched crawler:blocked")
	}
}
