package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type fakeStore struct {
	rows    []summary
	after   primitive.ObjectID
	limit   int64
	failure error
}

func (s *fakeStore) list(_ context.Context, after primitive.ObjectID, limit int64) ([]summary, error) {
	s.after, s.limit = after, limit
	return s.rows, s.failure
}

func (s *fakeStore) ready(context.Context) error { return s.failure }
func (s *fakeStore) metrics(context.Context) (pipelineMetrics, error) {
	ts := time.Unix(42, 0)
	return pipelineMetrics{Articles: 3, LatestScrape: &ts}, s.failure
}

func TestBoundedDataAndCORS(t *testing.T) {
	first, second, cursor := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
	store := &fakeStore{rows: []summary{{ID: first}, {ID: second}}}
	handler := newHandler(store)
	req := httptest.NewRequest(http.MethodGet, "/data?limit=1&cursor="+cursor.Hex(), nil)
	req.Header.Set("Origin", "http://localhost:8080")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK || store.after != cursor || store.limit != 2 ||
		w.Header().Get("X-Next-Cursor") != first.Hex() ||
		w.Header().Get("Cache-Control") != "no-store" ||
		w.Header().Get("Access-Control-Allow-Origin") != "http://localhost:8080" ||
		w.Header().Get("Access-Control-Expose-Headers") != "X-Next-Cursor" {
		t.Fatalf("bad pagination or headers: %v", w.Result())
	}
	if want := `[{"id":"` + first.Hex() + `"}]` + "\n"; w.Body.String() != want {
		t.Fatalf("body = %q; want %q", w.Body, want)
	}
	for _, path := range []string{"/data?limit=101", "/data?limit=0", "/data?limit=1&limit=2",
		"/data?cursor=", "/data?cursor=bad", "/data?cursor=%xx"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s status = %d", path, w.Code)
		}
	}
	store.rows = nil
	req = httptest.NewRequest(http.MethodGet, "/data", nil)
	req.Header.Set("Origin", "https://unrelated.invalid")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if store.limit != 26 || w.Body.String() != "[]\n" || w.Header().Get("Access-Control-Allow-Origin") != "" ||
		w.Header().Get("X-Next-Cursor") != "" {
		t.Fatalf("invalid empty page: %v %q", w.Result().Header, w.Body)
	}
}

func TestHealthAndErrors(t *testing.T) {
	store := &fakeStore{failure: errors.New("driver decision: secret details")}
	handler := newHandler(store)
	for path, expected := range map[string]int{"/data": 503, "/readyz": 503, "/metrics": 503, "/healthz": 204} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != expected || strings.Contains(w.Body.String(), "secret details") {
			t.Errorf("%s: %d %s", path, w.Code, w.Body)
		}
	}
	store.failure = nil
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if body := w.Body.String(); w.Code != 200 || !strings.Contains(body, "bwt_articles 3\n") ||
		!strings.Contains(body, "bwt_latest_scrape_timestamp_seconds 42\n") {
		t.Fatalf("metrics: %d %s", w.Code, body)
	}
}
