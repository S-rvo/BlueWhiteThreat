package sites

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/db"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/redis/go-redis/v9"
	"github.com/temoto/robotstxt"
)

type crawlProxyKey struct{}

const crawlAgent = "BlueWhiteThreat/1.0"

func crawlHost(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("invalid HTTP URL")
	}
	h := strings.ToLower(u.Hostname())
	if p := u.Port(); p != "" && p != "80" && p != "443" {
		h += ":" + p
	}
	return h, nil
}
func readRenderRobots(ctx context.Context, raw, proxy string) (*robotstxt.RobotsData, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	u.Path = "/robots.txt"
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	tr := &http.Transport{}
	if proxy != "" {
		p, e := url.Parse("socks5://" + proxy)
		if e != nil {
			return nil, e
		}
		tr.Proxy = http.ProxyURL(p)
	} else if strings.HasSuffix(u.Hostname(), ".onion") {
		return nil, fmt.Errorf("missing Tor proxy for robots")
	}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", crawlAgent)
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode == 429 || res.StatusCode >= 500 || res.StatusCode >= 300 && res.StatusCode < 400 {
		return nil, fmt.Errorf("robots unavailable: HTTP %d", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 256*1024+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 256*1024 {
		return nil, fmt.Errorf("robots exceeds limit")
	}
	return robotstxt.FromStatusAndBytes(res.StatusCode, data)
}

// Public rendering shares the crawler host lock. Resources are restricted to
// the initial origin, checked against robots, and paced; redirects to another
// origin are rediscovered by the HTTP frontier instead of followed here.
func preparePublicRender(ctx context.Context, target string) (func(), error) {
	host, err := crawlHost(target)
	if err != nil {
		return nil, err
	}
	origin, _ := url.Parse(target)
	client := db.ConnectRedis()
	closeRenderRedis := func() {
		if err := client.Close(); err != nil {
			log.Printf("close render Redis connection: %v", err)
		}
	}
	tokenBytes := make([]byte, 16)
	if _, err = rand.Read(tokenBytes); err != nil {
		closeRenderRedis()
		return nil, err
	}
	token := hex.EncodeToString(tokenBytes)
	var leaseUntil int64
	for {
		now := time.Now().Unix()
		leaseUntil = now + 180
		n, e := acquireRenderHost(ctx, client, host, token, now, leaseUntil)
		if e != nil {
			closeRenderRedis()
			return nil, e
		}
		if n == 1 {
			break
		}
		select {
		case <-ctx.Done():
			closeRenderRedis()
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	cooldown := int64(2)
	release := func() {
		c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		// If the lease expired, a subsequent crawler claim owns the host now.
		if err := releaseRenderHost(c, client, host, token, leaseUntil, time.Now().Unix()+cooldown); err != nil {
			log.Printf("release render host lease: %v", err)
		}
		closeRenderRedis()
	}
	proxy, _ := ctx.Value(crawlProxyKey{}).(string)
	robots, err := readRenderRobots(ctx, target, proxy)
	if err != nil {
		cooldown = 60
		release()
		return nil, err
	}
	if !robots.TestAgent(origin.RequestURI(), crawlAgent) {
		release()
		return nil, fmt.Errorf("%w: robots disallows page", errScrapePolicy)
	}
	if err = chromedp.Run(ctx); err != nil {
		release()
		return nil, err
	}
	listenCtx, cancelListen := context.WithCancel(ctx)
	var mu sync.Mutex
	delay := 2 * time.Second
	if group := robots.FindGroup(crawlAgent); group.CrawlDelay > delay {
		delay = group.CrawlDelay
	}
	cooldown = int64((delay + time.Second - 1) / time.Second)
	next := time.Now().Add(delay)
	chromedp.ListenTarget(listenCtx, func(ev interface{}) {
		paused, ok := ev.(*fetch.EventRequestPaused)
		if !ok {
			return
		}
		go func() {
			execCtx := cdp.WithExecutor(listenCtx, chromedp.FromContext(ctx).Target)
			u, e := url.Parse(paused.Request.URL)
			if e != nil || u.Scheme != origin.Scheme || !strings.EqualFold(u.Host, origin.Host) || !robots.TestAgent(u.RequestURI(), crawlAgent) {
				if err := fetch.FailRequest(paused.RequestID, network.ErrorReasonBlockedByClient).Do(execCtx); err != nil && listenCtx.Err() == nil {
					log.Print("could not block paused browser request")
				}
				return
			}
			mu.Lock()
			defer mu.Unlock()
			if wait := time.Until(next); wait > 0 {
				timer := time.NewTimer(wait)
				defer timer.Stop()
				select {
				case <-timer.C:
				case <-listenCtx.Done():
					return
				}
			}
			next = time.Now().Add(delay)
			if err := fetch.ContinueRequest(paused.RequestID).Do(execCtx); err != nil && listenCtx.Err() == nil {
				log.Print("could not continue paused browser request")
			}
		}()
	})
	err = chromedp.Run(ctx, emulation.SetUserAgentOverride(crawlAgent), network.SetBypassServiceWorker(true), network.SetCacheDisabled(true), network.SetBlockedURLs([]string{"ws://*", "wss://*"}), fetch.Enable())
	if err != nil {
		cancelListen()
		release()
		return nil, err
	}
	return func() {
		if err := chromedp.Run(ctx, fetch.Disable()); err != nil && ctx.Err() == nil {
			log.Print("could not disable browser interception")
		}
		cancelListen()
		release()
	}, nil
}

func acquireRenderHost(ctx context.Context, client *redis.Client, host, token string, now, until int64) (int, error) {
	return client.Eval(ctx, `
if tonumber(redis.call('HGET','crawler:host_ready',ARGV[1]) or '0')>tonumber(ARGV[2]) then return 0 end
redis.call('HSET','crawler:host_ready',ARGV[1],ARGV[3])
redis.call('HSET','crawler:render_host_owner',ARGV[1],ARGV[4])
redis.call('ZADD','crawler:hosts','XX',ARGV[3],ARGV[1])
return 1`, nil, host, now, until, token).Int()
}

func releaseRenderHost(ctx context.Context, client *redis.Client, host, token string, until, ready int64) error {
	return client.Eval(ctx, `
if redis.call('HGET','crawler:render_host_owner',ARGV[1])==ARGV[2] then
 redis.call('HDEL','crawler:render_host_owner',ARGV[1])
 if tonumber(redis.call('HGET','crawler:host_ready',ARGV[1]) or '0')==tonumber(ARGV[3]) then
  redis.call('HSET','crawler:host_ready',ARGV[1],ARGV[4])
  redis.call('ZADD','crawler:hosts','XX',ARGV[4],ARGV[1])
 end
end
return 1`, nil, host, token, until, ready).Err()
}
