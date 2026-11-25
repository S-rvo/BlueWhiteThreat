package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPublicFeedMetadata(t *testing.T) {
	cases := []struct {
		name, format, body string
		wantCount          int
		wantUTC            string
	}{
		{"RSS non trié", "rss", `<rss><channel>
		  <item><pubDate>Fri, 06 Nov 2026 15:00:00 CET</pubDate></item>
		  <item><pubDate>Fri, 06 Nov 2026 15:30:00 +0000</pubDate></item>
		  <item><pubDate>Sun, 01 Nov 2026 16:00:00 +0000</pubDate></item>
		</channel></rss>`, 3, "2026-11-06T15:30:00Z"},
		{"décalage CET", "rss", `<rss><channel>
		  <item><pubDate>Fri, 06 Nov 2026 15:00:00 CET</pubDate></item>
		</channel></rss>`, 1, "2026-11-06T14:00:00Z"},
		{"release KEV", "kev", `{"dateReleased":"2026-11-23T19:43:55.7979Z","count":1,"vulnerabilities":[{}]}`,
			1, "2026-11-23T19:43:55Z"},
		{"activité du dépôt", "github_repository", `{"full_name":"fastfire/deepdarkCTI","pushed_at":"2026-11-03T05:04:03Z"}`,
			0, "2026-11-03T05:04:03Z"},
		{"HTML vide", "rss", "<html></html>", 0, ""},
		{"RSS sans date", "rss", "<rss><channel><item/></channel></rss>", 0, ""},
		{"compte KEV incohérent", "kev", `{"dateReleased":"2026-11-23T19:43:55Z","count":2,"vulnerabilities":[{}]}`, 0, ""},
		{"dépôt différent", "github_repository", `{"full_name":"example/elsewhere","pushed_at":"2026-11-03T05:04:03Z"}`, 0, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n, date, err := feedMetadata(tc.format, []byte(tc.body))
			if tc.wantUTC == "" {
				if err == nil {
					t.Fatal("réponse inattendue acceptée")
				}
				return
			}
			if err != nil || n != tc.wantCount || date.UTC().Format(time.RFC3339) != tc.wantUTC {
				t.Fatalf("feedMetadata() = %d, %s, %v", n, date, err)
			}
		})
	}
}

func TestFeedStatusOnlyReadsConfiguredEndpoints(t *testing.T) {
	var unexpected atomic.Int32
	mux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rss":
			fmt.Fprintf(w, `<rss><channel><item>
				<link>http://%s/article</link>
				<pubDate>Fri, 06 Nov 2026 15:00:00 +0000</pubDate>
			</item></channel></rss>`, r.Host)
		case "/redirect":
			http.Redirect(w, r, "/forbidden", http.StatusFound)
		default:
			unexpected.Add(1)
		}
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	var out bytes.Buffer
	err := writeFeedStatus(&out, []publicFeed{
		{"redirect", server.URL + "/redirect", "rss"},
		{"fixture", server.URL + "/rss", "rss"},
	}, time.Unix(0, 0))
	if err == nil || unexpected.Load() != 0 {
		t.Fatalf("erreur attendue sans requête supplémentaire, err=%v, requêtes=%d", err, unexpected.Load())
	}
	var body struct{ Sources []feedStatus }
	if err := json.Unmarshal(out.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Sources) != 2 || body.Sources[0].Status != "error" ||
		body.Sources[1].Status != "ok" || body.Sources[1].Entries != 1 ||
		strings.Contains(out.String(), "/article") {
		t.Fatalf("rapport inattendu : %s", out.String())
	}
}
