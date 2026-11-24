package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tg "github.com/S-rvo/BlueWhiteThreat/apps/telegram/internal/telegram"
)

func TestRegistryRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "candidates.json")
	want := []tg.Candidate{{URL: "https://t.me/examplechannel", Kind: "public_candidate", State: "pending"}}
	if err := saveRegistry(path, want); err != nil {
		t.Fatal(err)
	}
	if err := saveRegistry(path, tg.Merge(want, want)); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got []tg.Candidate
	if err = json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].URL != want[0].URL {
		t.Fatal(got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("permissions %o", info.Mode().Perm())
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files remain: %v %v", entries, err)
	}
}

type testTransport func(*http.Request) (*http.Response, error)

func (f testTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestProgressSurvivesNetworkFailure(t *testing.T) {
	dir := t.TempDir()
	for _, file := range []string{"telegram_infostealer.md", "telegram_threat_actors.md"} {
		if err := os.WriteFile(filepath.Join(dir, file), []byte("| https://t.me/alpha_channel | ONLINE | Alpha |\n| https://t.me/beta_channel | ONLINE | Beta |\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(dir, "registry.json")
	calls := 0
	client := &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 2 {
			return nil, context.DeadlineExceeded
		}
		body := `<div class="tgme_widget_message" data-post="alpha_channel/1"><div class="tgme_widget_message_text">Report <a href="https://t.me/gamma_channel">source</a></div><time datetime="2026-10-23T10:00:00Z"></time></div>`
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	err := runWith([]string{"-seed-dir", dir, "-registry", path, "-probe", "-max-channels", "2", "-delay", "0s", "-request-timeout", "90s"}, func(string) (*http.Client, error) { return client, nil })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected timeout: %v", err)
	}
	if client.Timeout != 90*time.Second {
		t.Fatalf("timeout=%v", client.Timeout)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var candidates []tg.Candidate
	if err = json.Unmarshal(body, &candidates); err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 3 || candidates[0].State != "web_readable" || candidates[1].State != "check_failed" || candidates[0].CheckedAt == nil || candidates[1].CheckedAt == nil || candidates[2].URL != "https://t.me/gamma_channel" {
		t.Fatalf("progress lost: %+v", candidates)
	}
}

func TestFailedImportDoesNotReplaceRegistry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")
	original := []byte("[]\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	err := runWith([]string{"-seed-dir", dir, "-registry", path}, func(string) (*http.Client, error) { return &http.Client{}, nil })
	if err == nil {
		t.Fatal("expected import failure")
	}
	body, err := os.ReadFile(path)
	if err != nil || string(body) != string(original) {
		t.Fatalf("registry changed: %s %v", body, err)
	}
}

func TestFollowOnlyUsesRegistryAndWritesResumableOutbox(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")
	initial := []tg.Candidate{
		{URL: "https://t.me/alpha_channel", Kind: "public_candidate", State: "web_readable"},
		{URL: "https://t.me/beta_channel", Kind: "public_candidate", State: "pending"},
	}
	if err := saveRegistry(path, initial); err != nil {
		t.Fatal(err)
	}
	phase := 0
	requests := []string{}
	factory := func(string) (*http.Client, error) {
		return &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
			if r.URL.Host != "t.me" || r.URL.Path != "/s/alpha_channel" {
				t.Fatalf("follow visited discovery or GitHub: %s", r.URL)
			}
			requests = append(requests, r.URL.RawQuery)
			var body string
			if r.URL.Query().Get("before") == "3" {
				body = `<div class="tgme_widget_message" data-post="alpha_channel/2"><div class="tgme_widget_message_text">old</div><time datetime="2026-11-24T10:00:00Z"></time></div>`
			} else {
				body = `<div class="tgme_widget_message" data-post="alpha_channel/3"><div class="tgme_widget_message_text">new</div><time datetime="2026-11-24T10:00:00Z"></time></div><a class="tme_messages_more" data-before="3"></a>`
			}
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
		})}, nil
	}
	run := func() {
		t.Helper()
		if err := runWith([]string{"-mode", "follow", "-registry", path, "-max-pages", "1", "-delay", "0s", "-follow-interval", "1ns"}, factory); err != nil {
			t.Fatal(err)
		}
		phase++
	}
	run()
	load := func() []tg.Candidate {
		t.Helper()
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var out []tg.Candidate
		if err = json.Unmarshal(body, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	first := load()
	if first[0].BeforeID != 3 || first[0].LastMessageID != 0 {
		t.Fatal(first)
	}
	run()
	second := load()
	if second[0].BeforeID != 0 || second[0].LastMessageID != 3 || !second[0].Tracked {
		t.Fatal(second)
	}
	files, err := os.ReadDir(filepath.Join(dir, "outbox", "alpha_channel"))
	if err != nil || len(files) != 2 {
		t.Fatalf("outbox=%v %v", files, err)
	}
	// Third pass touches the latest page, but stores nothing at/below watermark.
	run()
	third := load()
	if third[0].LastMessageID != 3 || third[1].State != "pending" || len(requests) != 3 || requests[1] != "before=3" {
		t.Fatalf("registry=%+v requests=%v", third, requests)
	}
	if phase != 3 {
		t.Fatal(phase)
	}
}

func TestRepeatCancelsAndContinuesAfterCycleError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	count := 0
	err := repeat(ctx, time.Nanosecond, func() error {
		count++
		if count == 2 {
			cancel()
		}
		return errors.New("simulated Tor outage")
	})
	if err != nil || count != 2 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	if err = repeat(context.Background(), 0, func() error { return context.DeadlineExceeded }); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}

func TestRegistryLockPreventsConcurrentWriters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	unlock, err := lockRegistry(path)
	if err != nil {
		t.Fatal(err)
	}
	if other, e := lockRegistry(path); e == nil {
		other()
		unlock()
		t.Fatal("concurrent registry writer accepted")
	}
	unlock()
	again, err := lockRegistry(path)
	if err != nil {
		t.Fatal(err)
	}
	again()
}

func TestAllContinuesFollowWhenSeedRefreshFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")
	initial := []tg.Candidate{{URL: "https://t.me/alpha_channel", Kind: "public_candidate", State: "web_readable"}}
	if err := saveRegistry(path, initial); err != nil {
		t.Fatal(err)
	}
	calls := 0
	factory := func(string) (*http.Client, error) {
		return &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.URL.Host != "t.me" {
				t.Fatalf("expected follow, got %s", r.URL)
			}
			body := `<div class="tgme_widget_message" data-post="alpha_channel/1"><div class="tgme_widget_message_text">report</div><time datetime="2026-11-24T10:00:00Z"></time></div>`
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
		})}, nil
	}
	// Local seed files deliberately absent: existing follow queue must still run.
	if err := runWith([]string{"-mode", "all", "-seed-dir", dir, "-registry", path, "-delay", "0s"}, factory); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("requests=%d", calls)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []tg.Candidate
	if err = json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if out[0].LastMessageID != 1 {
		t.Fatal(out)
	}
}

func TestDiscoverDoesNotVisitTrackedSource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")
	for _, file := range []string{"telegram_infostealer.md", "telegram_threat_actors.md"} {
		if err := os.WriteFile(filepath.Join(dir, file), []byte("| https://t.me/alpha_channel | ONLINE | Alpha |\n| https://t.me/beta_channel | ONLINE | Beta |\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := saveRegistry(path, []tg.Candidate{{URL: "https://t.me/alpha_channel", Kind: "public_candidate", State: "web_readable"}}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	factory := func(string) (*http.Client, error) {
		return &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.URL.Path != "/s/beta_channel" {
				t.Fatalf("discovery visited tracked source %s", r.URL)
			}
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("<html>landing</html>")), Request: r}, nil
		})}, nil
	}
	if err := runWith([]string{"-mode", "discover", "-seed-dir", dir, "-registry", path, "-delay", "0s"}, factory); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("requests=%d", calls)
	}
}

func TestPaginationFailureDoesNotStopOtherChannels(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")
	for _, file := range []string{"telegram_infostealer.md", "telegram_threat_actors.md"} {
		if err := os.WriteFile(filepath.Join(dir, file), []byte("| https://t.me/gamma_channel | ONLINE | Gamma |\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := saveRegistry(path, []tg.Candidate{
		{URL: "https://t.me/alpha_channel", Kind: "public_candidate", Tracked: true, BeforeID: 765, CatchupHead: 780},
		{URL: "https://t.me/beta_channel", Kind: "public_candidate", Tracked: true},
	}); err != nil {
		t.Fatal(err)
	}
	var requests []string
	factory := func(string) (*http.Client, error) {
		return &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
			requests = append(requests, r.URL.Path)
			body := func(channel, id string) string {
				return `<div class="tgme_widget_message" data-post="` + channel + `/` + id + `"><div class="tgme_widget_message_text">report</div><time datetime="2026-11-24T10:00:00Z"></time></div>`
			}
			var html string
			switch r.URL.Path {
			case "/s/alpha_channel":
				html = body("alpha_channel", "742") + `<a class="tme_messages_more" data-before="not-a-number"></a>`
			case "/s/beta_channel":
				html = body("beta_channel", "3")
			case "/s/gamma_channel":
				html = body("gamma_channel", "9")
			default:
				t.Fatalf("unexpected request: %s", r.URL)
			}
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(html)), Request: r}, nil
		})}, nil
	}
	err := runWith([]string{"-mode", "all", "-seed-dir", dir, "-registry", path, "-delay", "0s", "-max-channels", "2"}, factory)
	if err != nil || strings.Join(requests, ",") != "/s/alpha_channel,/s/beta_channel,/s/gamma_channel" {
		t.Fatalf("cycle stopped at %v: %v", requests, err)
	}
	buf, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got []tg.Candidate
	if err = json.Unmarshal(buf, &got); err != nil {
		t.Fatal(err)
	}
	if got[0].State != "check_failed" || got[0].Failures != 1 || got[0].BeforeID != 765 || got[0].LastMessageID != 0 ||
		got[1].LastMessageID != 3 || !got[2].Tracked {
		t.Fatalf("failed to preserve channel progress and retry state: %+v", got)
	}
}

func TestDiscoveryDepthAndNewCandidateBudget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")
	const prefix = "https://t.me/"
	candidate := func(channel, parent string, tracked bool) tg.Candidate {
		c := tg.Candidate{URL: prefix + channel, Kind: "public_candidate", State: "pending", Tracked: tracked}
		if parent != "" {
			c.Provenance = []tg.Provenance{{Category: "message_link", Source: prefix + parent + "/1"}}
		}
		return c
	}
	if err := saveRegistry(path, []tg.Candidate{
		candidate("root_channel", "", true),
		candidate("child_channel", "root_channel", true),
		candidate("deep_channel", "child_channel", true),
		candidate("orphan_channel", "missing_channel", true),
	}); err != nil {
		t.Fatal(err)
	}
	var requests []string
	factory := func(string) (*http.Client, error) {
		return &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
			channel := strings.TrimPrefix(r.URL.Path, "/s/")
			requests = append(requests, channel)
			body := `<div class="tgme_widget_message" data-post="` + channel + `/1"><div class="tgme_widget_message_text">`
			if channel == "root_channel" {
				body += `<a href="https://t.me/new_alpha">first</a><a href="https://t.me/new_beta">second</a>`
			} else {
				body += `<a href="https://t.me/new_deep">deep</a>`
			}
			body += `</div><time datetime="2026-11-24T10:00:00Z"></time></div>`
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
		})}, nil
	}
	if err := runWith([]string{"-mode", "follow", "-registry", path, "-delay", "0s", "-max-discovery-depth", "1", "-max-new-candidates", "1"}, factory); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(requests, ","); got != "root_channel,child_channel" {
		t.Fatalf("requests = %s", got)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var result []tg.Candidate
	if err = json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if len(result) != 5 || result[4].URL != prefix+"new_alpha" || result[4].Provenance[0].Source != prefix+"root_channel/1" || !result[2].Tracked || result[2].LastMessageID != 0 {
		t.Fatalf("invalid registry: %+v", result)
	}
	raw, err = os.ReadFile(filepath.Join(dir, "outbox", "child_channel", "1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var article tg.Article
	if err = json.Unmarshal(raw, &article); err != nil || len(article.Links) != 1 || article.Links[0] != prefix+"new_deep" {
		t.Fatalf("stored article lost link: %+v %v", article, err)
	}
}
