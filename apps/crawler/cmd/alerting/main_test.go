package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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

func TestMaybeAlertSkipsWhenInCooldown(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	trySend := func(condition) (bool, error) { return false, nil }
	maybeAlert(server.URL, trySend, alert{condition: conditionQueueCapacity, message: "test"})

	if calls != 0 {
		t.Fatalf("expected no webhook call while in cooldown, got %d", calls)
	}
}

func TestMaybeAlertSendsWhenCooldownCleared(t *testing.T) {
	var received map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	trySend := func(condition) (bool, error) { return true, nil }
	maybeAlert(server.URL, trySend, alert{condition: conditionFreshness, message: "la file est en retard"})

	if received == nil || received["text"] != "la file est en retard" {
		t.Fatalf("expected webhook body with the alert message, got %+v", received)
	}
}

func TestMaybeAlertSkipsWebhookWhenURLEmpty(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
	}))
	defer server.Close()

	trySend := func(condition) (bool, error) { return true, nil }
	maybeAlert("", trySend, alert{condition: conditionQueueCapacity, message: "test"})

	if calls != 0 {
		t.Fatalf("expected no HTTP call when webhookURL is empty, got %d", calls)
	}
}

func TestMaybeAlertHandlesTrySendError(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
	}))
	defer server.Close()

	trySend := func(condition) (bool, error) { return false, errors.New("redis down") }
	maybeAlert(server.URL, trySend, alert{condition: conditionQueueCapacity, message: "test"})

	if calls != 0 {
		t.Fatalf("expected no webhook call when trySend errors, got %d", calls)
	}
}

func TestSendWebhookNonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	if err := sendWebhook(server.URL, "test"); err == nil {
		t.Fatal("expected an error for a non-2xx response")
	}
}
