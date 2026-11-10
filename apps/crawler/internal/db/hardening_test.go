package db

import (
	"fmt"
	"github.com/redis/go-redis/v9"
	"sync"
	"testing"
	"time"
)

func TestHostIdentityAndExclusion(t *testing.T) {
	setupTestRedisClient(t)
	for _, raw := range []string{"http://a.onion?x=1", "https://a.onion:443/?x=2", "http://a.onion:80/?x=3"} {
		host, err := HostKey(raw)
		if err != nil || host != "a.onion" {
			t.Fatalf("%s: %s %v", raw, host, err)
		}
		if _, err := AddURLToQueue(raw); err != nil {
			t.Fatal(err)
		}
	}
	u, _, err := ClaimURL()
	if err != nil || u == "" {
		t.Fatalf("%q %v", u, err)
	}
	u, _, err = ClaimURL()
	if err != nil || u != "" {
		t.Fatalf("concurrent same-host claim: %q %v", u, err)
	}
}
func TestStorageLimitIncludesDeferredAndMigrated(t *testing.T) {
	setupTestRedisClient(t)
	RedisClient.ZAdd(ctx, "crawler:scheduled", redis.Z{Score: float64(time.Now().Add(time.Hour).Unix()), Member: "http://a.onion/old"})
	for i := 0; i < 2; i++ {
		if err := ReconcileFrontier(); err != nil {
			t.Fatal(err)
		}
	}
	if n, _ := RedisClient.HGet(ctx, "crawler:stored_per_host", "a.onion").Int(); n != 1 {
		t.Fatalf("migration count %d", n)
	}
	RedisClient.HSet(ctx, "crawler:stored_per_host", "a.onion", 50000)
	reason, err := AddURLToQueue("http://a.onion/new")
	if err != nil || reason != "storage" {
		t.Fatalf("%s %v", reason, err)
	}
	if n, _ := RedisClient.ZCard(ctx, "crawler:scheduled").Result(); n != 1 {
		t.Fatal("cap admitted deferred URL")
	}
}
func TestRenderedPoisonDoesNotBlockAndDeadLetters(t *testing.T) {
	setupTestRedisClient(t)
	bad := `not-json`
	good := `{"Source":"http://a.onion","Links":["http://b.onion/x"]}`
	RedisClient.RPush(ctx, "crawler:rendered_links", bad, good)
	saved := 0
	save := func(string, []string) error { saved++; return nil }
	if err := drainRenderedLinks(save); err == nil {
		t.Fatal("poison error missing")
	}
	if saved != 1 {
		t.Fatal("head of line blocked")
	}
	for i := 0; i < 4; i++ {
		RedisClient.ZAdd(ctx, "crawler:rendered_retry", redis.Z{Score: 0, Member: bad})
		if err := drainRenderedLinks(save); err == nil {
			t.Fatal("poison error missing")
		}
	}
	if n, _ := RedisClient.LLen(ctx, "crawler:rendered_dead").Result(); n != 1 {
		t.Fatalf("dead count %d", n)
	}
}
func TestRenderedExpiredLeaseRecovered(t *testing.T) {
	setupTestRedisClient(t)
	raw := `{"Source":"http://a.onion","Links":["http://b.onion/x"]}`
	RedisClient.HSet(ctx, "crawler:rendered_inflight", "expired", raw)
	RedisClient.ZAdd(ctx, "crawler:rendered_leases", redis.Z{Score: 0, Member: "expired"})
	calls := 0
	if err := drainRenderedLinks(func(string, []string) error { calls++; return nil }); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal(fmt.Sprintf("recovered %d", calls))
	}
	if n, _ := RedisClient.HLen(ctx, "crawler:rendered_inflight").Result(); n != 0 {
		t.Fatal("lease leaked")
	}
}

func TestEmptyRenderedMessageIsRetriedNotStranded(t *testing.T) {
	setupTestRedisClient(t)
	RedisClient.RPush(ctx, "crawler:rendered_links", "")
	if err := drainRenderedLinks(func(string, []string) error { return nil }); err == nil {
		t.Fatal("empty payload accepted")
	}
	if n, _ := RedisClient.HLen(ctx, "crawler:rendered_inflight").Result(); n != 0 {
		t.Fatal("empty payload stranded")
	}
	if n, _ := RedisClient.ZCard(ctx, "crawler:rendered_retry").Result(); n != 1 {
		t.Fatal("empty payload not retried")
	}
}
func TestRenderedSourceCannotBypassStorageCap(t *testing.T) {
	setupTestRedisClient(t)
	RedisClient.HSet(ctx, "crawler:stored_per_host", "a.onion", 50000)
	RedisClient.RPush(ctx, "crawler:rendered_links", `{"Source":"http://a.onion/new","Links":["http://b.onion/x"]}`)
	writes := 0
	if err := drainRenderedLinks(func(string, []string) error { writes++; return nil }); err == nil {
		t.Fatal("source cap ignored")
	}
	if writes != 0 {
		t.Fatal("graph bypassed admission cap")
	}
}

func TestRenderedConcurrentClaims(t *testing.T) {
	setupTestRedisClient(t)
	for i := 0; i < 40; i++ {
		RedisClient.RPush(ctx, "crawler:rendered_links", fmt.Sprintf(`{"Source":"http://a.onion","Links":["http://b.onion/%d"]}`, i))
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	seen := map[string]int{}
	save := func(_ string, links []string) error {
		mu.Lock()
		defer mu.Unlock()
		for _, link := range links {
			seen[link]++
		}
		return nil
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := drainRenderedLinks(save); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if len(seen) != 40 {
		t.Fatalf("processed %d/40", len(seen))
	}
	for link, n := range seen {
		if n != 1 {
			t.Fatalf("duplicate live claim %s: %d", link, n)
		}
	}
	if n, _ := RedisClient.HLen(ctx, "crawler:rendered_inflight").Result(); n != 0 {
		t.Fatal("claims leaked")
	}
}
