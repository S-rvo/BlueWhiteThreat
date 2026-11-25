package sites

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/db"
	"github.com/chromedp/chromedp"
)

func TestPublicRenderPolicyFixture(t *testing.T) {
	addr, chrome := os.Getenv("CRAWLER_TEST_REDIS"), os.Getenv("CRAWLER_TEST_CHROME")
	if addr == "" || chrome == "" {
		t.Skip("requires disposable Redis and Chromium")
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("REDIS_HOST", host)
	t.Setenv("REDIS_PORT", port)
	t.Setenv("REDIS_PASSWORD", "")
	var denied, external atomic.Int32
	third := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { external.Add(1) }))
	defer third.Close()
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			fmt.Fprint(w, "User-agent: *\nDisallow: /private.js\n")
		case "/private.js":
			denied.Add(1)
		case "/allowed.js":
			w.Header().Set("Content-Type", "text/javascript")
			fmt.Fprint(w, `document.querySelector('main').textContent='rendered public fixture content';document.querySelector('a').href='/discovered';`)
		default:
			fmt.Fprintf(w, `<html><body><main>initial</main><a href='/original'>link</a><script src='/private.js'></script><script src='%s/foreign.js'></script><script src='/allowed.js'></script></body></html>`, third.URL)
		}
	}))
	defer page.Close()
	alloc, closeAlloc := chromedp.NewExecAllocator(context.Background(), append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(chrome), chromedp.NoSandbox)...)
	defer closeAlloc()
	browser, closeBrowser := chromedp.NewContext(alloc)
	defer closeBrowser()
	ctx, cancel := context.WithTimeout(browser, 45*time.Second)
	defer cancel()
	client := db.ConnectRedis()
	defer client.Close()
	key, _ := crawlHost(page.URL)
	// A simulated HTTP crawler lease delays the renderer before robots/network.
	client.HSet(ctx, "crawler:host_ready", key, time.Now().Add(2*time.Second).Unix())
	started := time.Now()
	content, err := FetchPage(ctx, page.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(started) < 4*time.Second {
		t.Fatal("host coordination/pacing missing")
	}
	if !strings.Contains(content, "rendered public") {
		t.Fatalf("JS did not render: %s", content)
	}
	if denied.Load() != 0 || external.Load() != 0 {
		t.Fatalf("blocked requests reached server: %d/%d", denied.Load(), external.Load())
	}
	raw, err := client.LIndex(ctx, "crawler:rendered_links", 0).Result()
	if err != nil || !strings.Contains(raw, "/discovered") {
		t.Fatalf("discovery %s %v", raw, err)
	}
	if n, _ := client.HExists(ctx, "crawler:render_host_owner", key).Result(); n {
		t.Fatal("host owner leaked")
	}
	t.Log("shared host lease, robots resource denial, third-party denial, JS execution and chunk handoff passed")
}
