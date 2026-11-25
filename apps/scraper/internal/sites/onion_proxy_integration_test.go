package sites

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// All SOCKS destinations are routed to the local fixture: no Tor/public traffic.
func TestOnionProxyRenderFixture(t *testing.T) {
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
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			fmt.Fprint(w, "User-agent: *\nAllow: /\n")
			return
		}
		fmt.Fprint(w, "<html><body><main>public rendered onion fixture content</main></body></html>")
	}))
	defer page.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			c, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				c.SetDeadline(time.Now().Add(30 * time.Second))
				h := make([]byte, 2)
				if _, err := io.ReadFull(c, h); err != nil {
					return
				}
				if _, err := io.CopyN(io.Discard, c, int64(h[1])); err != nil {
					return
				}
				c.Write([]byte{5, 0})
				req := make([]byte, 4)
				if _, err := io.ReadFull(c, req); err != nil {
					return
				}
				n := 0
				switch req[3] {
				case 1:
					n = 4
				case 4:
					n = 16
				case 3:
					b := make([]byte, 1)
					if _, err := io.ReadFull(c, b); err != nil {
						return
					}
					n = int(b[0])
				default:
					return
				}
				if _, err := io.CopyN(io.Discard, c, int64(n+2)); err != nil {
					return
				}
				upstream, err := net.DialTimeout("tcp", strings.TrimPrefix(page.URL, "http://"), time.Second)
				if err != nil {
					return
				}
				defer upstream.Close()
				c.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 80})
				done := make(chan struct{})
				go func() { io.Copy(upstream, c); upstream.Close(); close(done) }()
				io.Copy(c, upstream)
				c.Close()
				<-done
			}()
		}
	}()
	opts := append(BuildChromedpOptions(listener.Addr().String()), chromedp.ExecPath(chrome))
	alloc, closeAlloc := chromedp.NewExecAllocator(context.WithValue(context.Background(), crawlProxyKey{}, listener.Addr().String()), opts...)
	defer closeAlloc()
	browser, closeBrowser := chromedp.NewContext(alloc)
	defer closeBrowser()
	ctx, cancel := context.WithTimeout(browser, 30*time.Second)
	defer cancel()
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch e := ev.(type) {
		case *fetch.EventRequestPaused:
			t.Logf("paused %s", e.Request.URL)
		case *network.EventLoadingFailed:
			t.Logf("failed: %s %s", e.ErrorText, e.BlockedReason)
		}
	})
	content, err := FetchPage(ctx, "http://"+strings.Repeat("a", 56)+".onion/", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "fixture") {
		t.Fatal("missing fixture content")
	}
}
