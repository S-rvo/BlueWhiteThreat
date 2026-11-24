package telegram

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalize(t *testing.T) {
	cases := []struct{ raw, want, kind string }{
		{"https://t.me/s/ExampleChannel/42?before=43", "https://t.me/examplechannel", "public_candidate"},
		{"https://telegram.me/joinchat/Ab_C-12", "https://t.me/+Ab_C-12", "invite"},
		{"https://t.me/+AbCd12", "https://t.me/+AbCd12", "invite"},
	}
	for _, tc := range cases {
		got, kind, e := Normalize(tc.raw)
		if e != nil || got != tc.want || kind != tc.kind {
			t.Fatalf("%s: %s %s %v", tc.raw, got, kind, e)
		}
	}
	for _, raw := range []string{"https://evil.example/test", "https://t.me.evil.example/test", "https://t.me:443/test", "https://t.me/share/url", "https://t.me/c/123/456", "https://t.me/name/not-a-post", "https://t.me/+"} {
		if _, _, e := Normalize(raw); e == nil {
			t.Errorf("accepted %s", raw)
		}
	}
}
func TestImportMerge(t *testing.T) {
	input := "| Telegram | Status | Name |\n|---|---|---|\n| [A](https://t.me/ExampleChannel) | ONLINE | Actor | DDoS |\nhttps://t.me/+AbCd | VALID | Invite\n"
	a, e := ParseSeeds(strings.NewReader(input), "telegram_threat_actors.md")
	if e != nil || len(a) != 2 {
		t.Fatalf("%+v %v", a, e)
	}
	b, e := ParseSeeds(strings.NewReader(input), "telegram_infostealer.md")
	if e != nil {
		t.Fatal(e)
	}
	merged := Merge(a, b, a)
	if len(merged) != 2 || len(merged[0].Provenance) != 2 || merged[1].State != "invitation_required" {
		t.Fatalf("%+v", merged)
	}
}
func post(channel string, id int, text string) string {
	return fmt.Sprintf(`<div class="tgme_widget_message" data-post="%s/%d"><div class="tgme_widget_message_text">%s</div><time datetime="2026-10-23T10:00:00+00:00"></time></div>`, channel, id, text)
}
func TestPreview(t *testing.T) {
	body := post("ExampleChannel", 1, `hello<br>world <a href="https://t.me/OtherChannel/7">source</a>`) + post("ExampleChannel", 2, "newer") + post("OtherChannel", 3, "wrong channel")
	state, a, e := ParsePreview([]byte(body), "https://t.me/examplechannel", 20)
	if e != nil || state != "web_readable" || len(a) != 2 || !strings.Contains(a[0].Content, "hello\nworld") || len(a[0].Links) != 1 {
		t.Fatalf("%s %+v %v", state, a, e)
	}
	_, a, _ = ParsePreview([]byte(body), "https://t.me/examplechannel", 1)
	if len(a) != 1 || a[0].MessageID != 2 {
		t.Fatal(a)
	}
	state, a, _ = ParsePreview([]byte("<h1>Channel</h1><button>View in Telegram</button>"), "https://t.me/examplechannel", 20)
	if state != "preview_unavailable" || len(a) != 0 {
		t.Fatal(state)
	}
	state, a, _ = ParsePreview([]byte(post("ExampleChannel", 1, "child porn")), "https://t.me/examplechannel", 20)
	if state != "blocked_content" || len(a) != 0 {
		t.Fatal(state)
	}
}
func TestFetchDoesNotFollowRedirect(t *testing.T) {
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/target" {
			hits++
			return
		}
		http.Redirect(w, r, "/target", 302)
	}))
	defer server.Close()
	if _, e := Fetch(context.Background(), server.Client(), server.URL); e == nil || hits != 0 {
		t.Fatalf("redirect followed: %v hits=%d", e, hits)
	}
}
func TestFetchBoundAndStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/error" {
			w.WriteHeader(429)
			return
		}
		fmt.Fprint(w, strings.Repeat("x", (4<<20)+1))
	}))
	defer server.Close()
	for _, path := range []string{"/error", "/large"} {
		if _, e := Fetch(context.Background(), server.Client(), server.URL+path); e == nil {
			t.Fatal("accepted", path)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestProbeRedirectDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		location, state string
		wantError       bool
	}{
		{"https://t.me/examplechannel", "preview_unavailable", false},
		{"/examplechannel", "preview_unavailable", false},
		{"https://other.example/landing", "check_failed", true},
	} {
		t.Run(tc.location, func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{tc.location}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
			})}
			c := Candidate{URL: "https://t.me/examplechannel", Kind: "public_candidate"}
			articles, err := Probe(context.Background(), client, &c, 10)
			if (err != nil) != tc.wantError || c.State != tc.state || len(articles) != 0 || calls != 1 || !strings.Contains(c.Error, "HTTP 302") {
				t.Fatalf("candidate=%+v err=%v calls=%d", c, err, calls)
			}
		})
	}
}
