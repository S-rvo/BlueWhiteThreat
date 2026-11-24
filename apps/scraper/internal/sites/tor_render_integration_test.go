package sites

import (
	"context"
	"encoding/json"
	"github.com/chromedp/chromedp"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/db"
)

func TestTorRenderedFixture(t *testing.T) {
	target, socks, redisAddr := os.Getenv("CRAWLER_TEST_ONION"), os.Getenv("CRAWLER_TEST_SOCKS_HOST"), os.Getenv("CRAWLER_TEST_REDIS")
	if target == "" || socks == "" || redisAddr == "" {
		t.Skip("requires disposable Tor and Redis fixtures")
	}
	host, port, err := net.SplitHostPort(redisAddr)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("REDIS_HOST", host)
	t.Setenv("REDIS_PORT", port)
	t.Setenv("REDIS_PASSWORD", "")
	opts := BuildChromedpOptions(socks)
	if executable := os.Getenv("CRAWLER_TEST_CHROME"); executable != "" {
		opts = append(opts, chromedp.ExecPath(executable))
	}
	allocator, closeAllocator := chromedp.NewExecAllocator(context.WithValue(context.Background(), crawlProxyKey{}, socks), opts...)
	defer closeAllocator()
	browser, closeBrowser := chromedp.NewContext(allocator)
	defer closeBrowser()
	ctx, cancel := context.WithTimeout(browser, 2*time.Minute)
	defer cancel()
	start := time.Now()
	content, err := FetchPage(ctx, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "rendered") {
		t.Fatalf("missing JS content: %q", content)
	}
	client := db.ConnectRedis()
	defer client.Close()
	raw, err := client.LIndex(ctx, "crawler:rendered_links", 0).Result()
	if err != nil {
		t.Fatal(err)
	}
	var event struct {
		Source string
		Links  []string
	}
	if err := json.Unmarshal([]byte(raw), &event); err != nil {
		t.Fatal(err)
	}
	if event.Source != target || len(event.Links) != 1 || event.Links[0] != strings.TrimRight(target, "/")+"/javascript-link" {
		t.Fatalf("%+v", event)
	}
	t.Logf("Tor + Chromium JS rendering + Redis handoff: %s", time.Since(start))
}
