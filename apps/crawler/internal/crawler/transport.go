package crawler

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const responseLimit = 2 * 1024 * 1024

type robotsEntry struct {
	body    []byte
	header  http.Header
	status  int
	expires time.Time
}

var robotsCache = struct {
	sync.Mutex
	entries map[string]robotsEntry
}{entries: make(map[string]robotsEntry)}

type boundedTransport struct {
	ctx      context.Context
	base     http.RoundTripper
	sitemaps *[]string
}

func (t *boundedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.ctx != nil {
		req = req.Clone(t.ctx)
	}
	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	robots := req.URL.Path == "/robots.txt" && req.URL.RawQuery == ""
	key := req.URL.String()
	if robots {
		robotsCache.Lock()
		entry, ok := robotsCache.entries[key]
		robotsCache.Unlock()
		if ok && time.Now().Before(entry.expires) {
			if entry.status >= 200 && entry.status < 300 {
				t.discover(entry.body)
			}
			return &http.Response{StatusCode: entry.status, Header: entry.header.Clone(), Body: io.NopCloser(bytes.NewReader(entry.body)), Request: req}, nil
		}
	}
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" || strings.HasSuffix(req.URL.Path, ".gz") && !resp.Uncompressed {
		gz, err := gzip.NewReader(reader)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		reader = gz
		resp.Header.Del("Content-Encoding")
		resp.Uncompressed = true
	}
	body, err := io.ReadAll(io.LimitReader(reader, responseLimit+1))
	if err != nil {
		return nil, err
	}
	if len(body) > responseLimit {
		return nil, fmt.Errorf("response exceeds %d decoded bytes", responseLimit)
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.Header.Del("Content-Length")
	if strings.HasSuffix(req.URL.Path, ".xml.gz") {
		resp.Header.Set("Content-Type", "application/xml")
	}
	if robots {
		// Transient failures must reach the retry scheduler, never become an allow-all cache entry.
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			return nil, fmt.Errorf("robots temporarily unavailable: HTTP %d", resp.StatusCode)
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 || resp.StatusCode == 404 || resp.StatusCode == 410 {
			robotsCache.Lock()
			if len(robotsCache.entries) >= 2048 {
				for k, v := range robotsCache.entries {
					if time.Now().After(v.expires) {
						delete(robotsCache.entries, k)
					}
				}
				if len(robotsCache.entries) >= 2048 {
					for k := range robotsCache.entries {
						delete(robotsCache.entries, k)
						break
					}
				}
			}
			robotsCache.entries[key] = robotsEntry{body, resp.Header.Clone(), resp.StatusCode, time.Now().Add(time.Hour)}
			robotsCache.Unlock()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				t.discover(body)
			}
		}
	}
	return resp, nil
}
func (t *boundedTransport) discover(body []byte) {
	for _, line := range strings.Split(string(body), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if ok && strings.EqualFold(strings.TrimSpace(key), "Sitemap") {
			value, _, _ = strings.Cut(value, "#")
			*t.sitemaps = append(*t.sitemaps, strings.TrimSpace(value))
		}
	}
}
