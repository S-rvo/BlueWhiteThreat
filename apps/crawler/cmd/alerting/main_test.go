package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestEvaluateThresholds(t *testing.T) {
	cases := []struct {
		name       string
		queueRatio float64
		oldestAge  time.Duration
		hasAge     bool
		want       []condition
	}{
		{"nothing triggered", 0.5, 2 * 24 * time.Hour, true, nil},
		{"queue capacity only", 0.95, 2 * 24 * time.Hour, true, []condition{conditionQueueCapacity}},
		{"freshness only", 0.1, 15 * 24 * time.Hour, true, []condition{conditionFreshness}},
		{"both", 0.95, 15 * 24 * time.Hour, true, []condition{conditionQueueCapacity, conditionFreshness}},
		{"no age data available", 0.1, 15 * 24 * time.Hour, false, nil},
		{"exactly at queue threshold", queueCapacityWarnRatio, 0, true, []condition{conditionQueueCapacity}},
		{"exactly at freshness threshold (not strictly greater)", 0, staleAgeWarnThreshold, true, nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := evaluateThresholds(c.queueRatio, c.oldestAge, c.hasAge)
			if len(got) != len(c.want) {
				t.Fatalf("got %d alerts, want %d (%+v)", len(got), len(c.want), got)
			}
			for i, a := range got {
				if a.condition != c.want[i] {
					t.Errorf("alert %d: got condition %q, want %q", i, a.condition, c.want[i])
				}
			}
		})
	}
}

func TestMaybeAlertCooldownAndErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		send bool
		err  error
		want int
	}{
		{"clear", true, nil, 1},
		{"cooldown", false, nil, 0},
		{"storage error", false, errors.New("redis down"), 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			notify := func(message string) error {
				calls++
				if message != "test" {
					t.Errorf("message = %q", message)
				}
				return nil
			}
			trySend := func(condition) (bool, error) { return tc.send, tc.err }
			maybeAlert(notify, trySend, alert{condition: conditionQueueCapacity, message: "test"})
			if calls != tc.want {
				t.Fatalf("got %d calls, want %d", calls, tc.want)
			}
		})
	}
}

type testTransport func(*http.Request) (*http.Response, error)

func (t testTransport) RoundTrip(r *http.Request) (*http.Response, error) { return t(r) }

func TestWebhookDestinationAndRedirectPolicy(t *testing.T) {
	const path = "/services/fixtureA/fixtureB/fixtureC"
	valid := "https://hooks.slack.com" + path
	if got, err := slackPath(valid); err != nil || got != path {
		t.Fatalf("valid fixture rejected: %v", err)
	}
	for _, raw := range []string{
		"http://hooks.slack.com" + path,
		"https://unrelated.invalid" + path,
		"https://hooks.slack.com:443" + path,
		valid + "?other=value", valid + "#fragment",
		"https://hooks.slack.com/not-services",
	} {
		if _, err := slackPath(raw); err == nil {
			t.Error("unexpected webhook accepted")
		}
	}
	client := webhookClient()
	calls := 0
	client.Transport = testTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		var received map[string]string
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil ||
			received["text"] != "test" || r.URL.String() != valid || r.Method != http.MethodPost {
			t.Error("incorrect outbound Slack request")
		}
		return &http.Response{StatusCode: http.StatusFound, Body: io.NopCloser(strings.NewReader("")),
			Header: http.Header{"Location": {"https://unrelated.invalid"}}}, nil
	})
	if err := sendWebhook(client, path, "test"); err == nil || calls != 1 {
		t.Fatalf("redirect should be rejected: calls=%d err=%v", calls, err)
	}
}
