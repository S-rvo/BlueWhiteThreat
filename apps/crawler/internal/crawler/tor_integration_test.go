package crawler

import (
	"context"
	"github.com/S-rvo/BlueWhiteThreat/apps/crawler/internal/db"
	"github.com/gocolly/colly/proxy"
	"github.com/redis/go-redis/v9"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"
)

// Use an operator-selected onion fixture, never the production seed corpus.
// At most five sequential requests with a two-second pause.
func TestTorFixture(t *testing.T) {
	target, socks := os.Getenv("CRAWLER_TEST_ONION"), os.Getenv("CRAWLER_TEST_SOCKS")
	if target == "" || socks == "" {
		t.Skip("set CRAWLER_TEST_ONION and CRAWLER_TEST_SOCKS for a controlled Tor fixture")
	}
	addr := os.Getenv("CRAWLER_TEST_REDIS")
	if addr == "" {
		t.Fatal("CRAWLER_TEST_REDIS must point to disposable Redis")
	}
	db.RedisClient = redis.NewClient(&redis.Options{Addr: addr})
	defer func() { db.RedisClient.Close(); db.RedisClient = nil }()
	if err := db.RedisClient.Ping(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	if !allowed(target) {
		t.Fatal("expected an allowed onion fixture")
	}
	switcher, err := proxy.RoundRobinProxySwitcher(socks)
	if err != nil {
		t.Fatal(err)
	}
	rp := func(r *http.Request) (*url.URL, error) { return switcher(r.Clone(r.Context())) }
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	start := time.Now()
	for i := 0; i < 5; i++ {
		result, err := fetchContext(ctx, target, nil, rp, allowed)
		if err != nil || result.Status < 200 || result.Status >= 300 {
			t.Fatalf("request %d: status=%d err=%v", i, result.Status, err)
		}
		if i < 4 {
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-time.After(2 * time.Second):
			}
		}
	}
	t.Logf("five sequential Tor fetches (including pacing): %s", time.Since(start))
}
