package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHarvesterPreservesClientTimeout(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer s.Close()
	client := &http.Client{Timeout: 50 * time.Millisecond, Transport: &onionLocationHarvester{ctx: context.Background(), base: http.DefaultTransport, found: map[string]bool{}}}
	start := time.Now()
	resp, err := client.Get(s.URL)
	if resp != nil {
		resp.Body.Close()
	}
	if err == nil || time.Since(start) > time.Second {
		t.Fatalf("timeout not preserved: %v", err)
	}
}
