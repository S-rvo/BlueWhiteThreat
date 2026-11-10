package crawler

import (
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchLocalWeb(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "User-agent: *\nDisallow: /private\n") })
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", "v1")
		fmt.Fprint(w, `<a href="/page#x">a</a><a href="/page#y">b</a><a href="http://outside.invalid/">outside</a>`)
	})
	mux.HandleFunc("/private", func(w http.ResponseWriter, r *http.Request) { t.Error("robots denied page requested") })
	mux.HandleFunc("/limited", func(w http.ResponseWriter, r *http.Request) { w.Header().Set("Retry-After", "120"); w.WriteHeader(429) })
	mux.HandleFunc("/cached", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") != "v1" {
			t.Error("missing validator")
		}
		w.WriteHeader(304)
	})
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "http://outside.invalid/", 302) })
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, `<urlset><url><loc>/from-sitemap</loc></url></urlset>`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	accept := func(raw string) bool { return len(raw) >= len(server.URL) && raw[:len(server.URL)] == server.URL }
	r, err := fetch(server.URL, nil, nil, accept)
	if err != nil || len(r.Links) != 2 || r.Links[0] != server.URL+"/page" || r.ETag != "v1" {
		t.Fatalf("%+v %v", r, err)
	}
	sm, se := fetch(server.URL+"/sitemap.xml", nil, nil, accept)
	if se != nil || len(sm.Links) != 1 || sm.Links[0] != server.URL+"/from-sitemap" {
		t.Fatalf("sitemap: %+v %v", sm, se)
	}
	r, err = fetch(server.URL+"/private", nil, nil, accept)
	if err != nil || !r.Denied {
		t.Fatalf("robots: %+v %v", r, err)
	}
	r, err = fetch(server.URL+"/limited", nil, nil, accept)
	if err == nil || r.Status != 429 || r.RetryAfter != 2*time.Minute {
		t.Fatalf("429: %+v %v", r, err)
	}
	r, err = fetch(server.URL+"/cached", map[string]string{"etag": "v1"}, nil, accept)
	if err != nil || r.Status != 304 {
		t.Fatalf("304: %+v %v", r, err)
	}
	_, err = fetch(server.URL+"/redirect", nil, nil, accept)
	if err == nil {
		t.Fatal("external redirect accepted")
	}
}

func TestPlainTextAndHeaderOnionDiscovery(t *testing.T) {
	bodyOnion := strings.Repeat("a", 56) + ".onion"
	headerOnion := strings.Repeat("b", 56) + ".onion"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			return
		}
		w.Header().Set("Onion-Location", "http://"+headerOnion+"/")
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, "no hyperlink, just text: join %s today", bodyOnion)
	}))
	defer server.Close()
	result, err := fetch(server.URL+"/forum", nil, nil, func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if !containsLink(result.Links, "http://"+bodyOnion) {
		t.Errorf("plain-text onion not discovered: %v", result.Links)
	}
	if !containsLink(result.Links, "http://"+headerOnion) {
		t.Errorf("Onion-Location mirror not discovered: %v", result.Links)
	}
}

func containsLink(links []string, want string) bool {
	for _, l := range links {
		if l == want {
			return true
		}
	}
	return false
}

func TestRobotsCacheAndSitemapDiscovery(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			requests++
			fmt.Fprintf(w, "User-agent: *\nSitemap: http://%s/map.xml\n", r.Host)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html>ok</html>")
	}))
	defer server.Close()
	accept := func(raw string) bool { return strings.HasPrefix(raw, server.URL+"/") }
	for i := 0; i < 2; i++ {
		result, err := fetch(server.URL+"/page", nil, nil, accept)
		if err != nil || len(result.Links) != 1 || result.Links[0] != server.URL+"/map.xml" {
			t.Fatalf("%+v %v", result, err)
		}
	}
	if requests != 1 {
		t.Fatalf("robots requests: %d", requests)
	}
}
func TestOversizedResponseRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			return
		}
		fmt.Fprint(w, strings.Repeat("x", responseLimit+1))
	}))
	defer server.Close()
	_, err := fetch(server.URL+"/page", nil, nil, func(string) bool { return true })
	if err == nil {
		t.Fatal("oversized response accepted")
	}
}

func TestFetchCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("cancelled request reached server") }))
	defer server.Close()
	_, err := fetchContext(ctx, server.URL, nil, nil, func(string) bool { return true })
	if err == nil {
		t.Fatal("cancelled fetch accepted")
	}
}

func TestGzipSitemap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		gz := gzip.NewWriter(w)
		fmt.Fprintf(gz, "<urlset><url><loc>http://%s/page</loc></url></urlset>", r.Host)
		gz.Close()
	}))
	defer server.Close()
	result, err := fetch(server.URL+"/sitemap.xml.gz", nil, nil, func(string) bool { return true })
	if err != nil || len(result.Links) != 1 || result.Links[0] != server.URL+"/page" {
		t.Fatalf("%+v %v", result, err)
	}
}

func TestCrossHostRedirectBecomesDiscovery(t *testing.T) {
	targetRequests := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetRequests++ }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			return
		}
		http.Redirect(w, r, target.URL+"/page", http.StatusMovedPermanently)
	}))
	defer source.Close()
	result, err := fetch(source.URL+"/old", nil, nil, func(string) bool { return true })
	if err != nil || result.Status != 301 || len(result.Links) != 1 || result.Links[0] != target.URL+"/page" || targetRequests != 0 {
		t.Fatalf("%+v %v requests=%d", result, err, targetRequests)
	}
}

func TestRobotsRetryAfter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/robots.txt" {
			t.Error("page requested")
		}
		w.Header().Set("Retry-After", "600")
		w.WriteHeader(503)
	}))
	defer server.Close()
	result, err := fetch(server.URL, nil, nil, func(string) bool { return true })
	if err == nil || result.Status != 503 || result.RetryAfter != 10*time.Minute {
		t.Fatalf("%+v %v", result, err)
	}
}
func TestInternalRedirectIsSeparateTask(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			fmt.Fprint(w, "User-agent: *\nDisallow: /private\n")
		case "/start":
			http.Redirect(w, r, "/private", 302)
		case "/private":
			t.Error("forbidden path fetched")
		}
	}))
	defer server.Close()
	result, err := fetch(server.URL+"/start", nil, nil, func(string) bool { return true })
	if err != nil || len(result.Links) != 1 {
		t.Fatalf("%+v %v", result, err)
	}
	result, err = fetch(result.Links[0], nil, nil, func(string) bool { return true })
	if err != nil || !result.Denied {
		t.Fatalf("%+v %v", result, err)
	}
}
