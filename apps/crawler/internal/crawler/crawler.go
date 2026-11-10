package crawler

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/crawler/internal/db"
	"github.com/S-rvo/BlueWhiteThreat/pkg/csamfilter"
	"github.com/gocolly/colly"
	"github.com/gocolly/colly/proxy"
)

type Result struct {
	Links                []string
	Status               int
	ETag, Modified, Hash string
	RetryAfter           time.Duration
	Denied               bool
}

func allowed(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.User == nil && (u.Scheme == "http" || u.Scheme == "https") && strings.HasSuffix(strings.ToLower(u.Hostname()), ".onion") && !csamfilter.IsBlockedURL(raw) && !db.IsFarmBackend(raw)
}

// Fetch processes exactly one page. Discovery is persisted by the caller;
// there is no recursive fan-out or shared mutable response status.
var proxyOnce sync.Once
var sharedProxy colly.ProxyFunc
var proxyError error
var proxyMutex sync.Mutex

func Fetch(raw string, metadata map[string]string) (Result, error) {
	return FetchContext(context.Background(), raw, metadata)
}
func FetchContext(ctx context.Context, raw string, metadata map[string]string) (Result, error) {
	proxyOnce.Do(func() {
		sharedProxy, proxyError = proxy.RoundRobinProxySwitcher("socks5://tor1:9050", "socks5://tor2:9050", "socks5://tor3:9050")
	})
	if proxyError != nil {
		return Result{}, proxyError
	}
	// Colly v1's round-robin reads its index outside an atomic operation.
	rp := func(r *http.Request) (*url.URL, error) {
		proxyMutex.Lock()
		defer proxyMutex.Unlock()
		return sharedProxy(r)
	}
	return fetchContext(ctx, raw, metadata, rp, allowed)
}

func fetch(raw string, metadata map[string]string, proxyFn colly.ProxyFunc, accept func(string) bool) (Result, error) {
	return fetchContext(context.Background(), raw, metadata, proxyFn, accept)
}
func fetchContext(ctx context.Context, raw string, metadata map[string]string, proxyFn colly.ProxyFunc, accept func(string) bool) (Result, error) {
	var result Result
	if !accept(raw) {
		return result, fmt.Errorf("invalid or blocked URL")
	}
	c := colly.NewCollector(colly.UserAgent("BlueWhiteThreat/1.0"))
	c.IgnoreRobotsTxt = false
	c.MaxBodySize = responseLimit
	c.SetRequestTimeout(45 * time.Second)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = proxyFn
	defer transport.CloseIdleConnections()
	var robotSitemaps []string
	c.WithTransport(&boundedTransport{base: transport, sitemaps: &robotSitemaps, ctx: ctx})
	c.RedirectHandler = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 || !accept(req.URL.String()) || (len(via) > 0 && !strings.EqualFold(req.URL.Host, via[0].URL.Host)) {
			return fmt.Errorf("redirect rejected")
		}
		// Conditional validators belong to the original resource only.
		req.Header.Del("If-None-Match")
		req.Header.Del("If-Modified-Since")
		return nil
	}
	c.OnRequest(func(r *colly.Request) {
		if metadata["etag"] != "" {
			r.Headers.Set("If-None-Match", metadata["etag"])
		}
		if metadata["modified"] != "" {
			r.Headers.Set("If-Modified-Since", metadata["modified"])
		}
	})
	record := func(r *colly.Response) {
		result.Status = r.StatusCode
		result.ETag = r.Headers.Get("ETag")
		result.Modified = r.Headers.Get("Last-Modified")
		result.Hash = fmt.Sprintf("%x", sha256.Sum256(r.Body))
		result.RetryAfter = retryAfter(r.Headers.Get("Retry-After"), time.Now())
	}
	c.OnResponse(record)
	c.OnError(func(r *colly.Response, err error) {
		if r.Headers != nil {
			record(r)
		}
	})
	seen := map[string]bool{}
	addLink := func(link string) {
		u, err := url.Parse(link)
		if err != nil {
			return
		}
		u.Fragment = ""
		link = u.String()
		if len(result.Links) < 5000 && !seen[link] && accept(link) {
			seen[link] = true
			result.Links = append(result.Links, link)
		}
	}
	c.OnHTML("a[href]", func(e *colly.HTMLElement) { addLink(e.Request.AbsoluteURL(e.Attr("href"))) })
	// Sitemap indexes and URL sets enter the same durable frontier as HTML links.
	c.OnXML("//*[local-name()='loc']", func(e *colly.XMLElement) { addLink(e.Request.AbsoluteURL(strings.TrimSpace(e.Text))) })

	err := c.Visit(raw)
	for _, sitemap := range robotSitemaps {
		addLink(sitemap)
	}
	if err == colly.ErrRobotsTxtBlocked {
		result.Denied = true
		return result, nil
	}
	if result.Status == 304 || (result.Status >= 400 && result.Status < 500 && result.Status != 429 && result.Status != 408) {
		return result, nil
	}
	if err == nil && result.Status >= 200 && result.Status < 300 {
		if u, e := url.Parse(raw); e == nil && (u.Path == "" || u.Path == "/") {
			addLink(u.Scheme + "://" + u.Host + "/sitemap.xml")
		}
	}
	return result, err
}
func retryAfter(value string, now time.Time) time.Duration {
	var d time.Duration
	if n, err := strconv.Atoi(value); err == nil && n > 0 {
		if n > 86400 {
			n = 86400
		}
		d = time.Duration(n) * time.Second
	} else if t, err := http.ParseTime(value); err == nil {
		d = t.Sub(now)
	}
	if d < 0 {
		return 0
	}
	if d > 24*time.Hour {
		return 24 * time.Hour
	}
	return d
}

// Crawler retains the historical API; depth is handled by the durable frontier.
func Crawler(raw string, depth int) ([]string, []string, int, error) {
	r, err := Fetch(raw, nil)
	return []string{raw}, r.Links, r.Status, err
}
