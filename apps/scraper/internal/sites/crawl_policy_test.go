package sites

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRenderedChunksLossless(t *testing.T) {
	links := make([]string, 5000)
	for i := range links {
		links[i] = "http://a.onion/" + strings.Repeat("x", 150)
	}
	chunks, err := renderedChunks("http://a.onion", links)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) < 2 {
		t.Fatal("not chunked")
	}
	count := 0
	for _, chunk := range chunks {
		if len(chunk) > 64*1024 {
			t.Fatal("oversized")
		}
		var e struct {
			Source string
			Links  []string
		}
		if err := json.Unmarshal([]byte(chunk), &e); err != nil {
			t.Fatal(err)
		}
		count += len(e.Links)
	}
	if count != len(links) {
		t.Fatalf("lost links: %d", count)
	}
}
func TestRenderRobots(t *testing.T) {
	for _, tc := range []struct {
		status  int
		body    string
		wantErr bool
	}{{200, "User-agent: *\nDisallow: /private", false}, {404, "", false}, {429, "", true}, {503, "", true}, {302, "", true}, {200, strings.Repeat("x", 256*1024+1), true}} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/robots.txt" || r.UserAgent() != crawlAgent {
				t.Errorf("request %v", r)
			}
			w.WriteHeader(tc.status)
			w.Write([]byte(tc.body))
		}))
		robots, err := readRenderRobots(context.Background(), server.URL+"/page?x=1", "")
		server.Close()
		if (err != nil) != tc.wantErr {
			t.Fatalf("status %d: %v", tc.status, err)
		}
		if err == nil && tc.status == 200 && robots.TestAgent("/private", crawlAgent) {
			t.Fatal("disallow ignored")
		}
	}
}
