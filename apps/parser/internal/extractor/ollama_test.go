package extractor

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOllamaExtractor_ParsesStructuredResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		resp := map[string]any{
			"message": map[string]any{
				"role": "assistant",
				"content": `{"iocs":{"ips":["1.2.3.4"],"hashes":[],"domains":[],"urls":[]},` +
					`"cves":["CVE-2024-1234"],"malware":["Emotet"],"threat_actors":[],"infrastructure":[]}`,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	ext := NewOllamaExtractor(srv.URL, "qwen2.5:14b-instruct")
	out, err := ext.Extract(context.Background(), "some scraped article text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.IOCs.IPs) != 1 || out.IOCs.IPs[0] != "1.2.3.4" {
		t.Fatalf("expected IP extracted, got %v", out.IOCs.IPs)
	}
	if len(out.Malware) != 1 || out.Malware[0] != "Emotet" {
		t.Fatalf("expected malware extracted, got %v", out.Malware)
	}
}

func TestOllamaExtractor_ErrorsOnInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"message": map[string]any{"role": "assistant", "content": "not json"},
		})
	}))
	defer srv.Close()

	ext := NewOllamaExtractor(srv.URL, "qwen2.5:14b-instruct")
	_, err := ext.Extract(context.Background(), "text")
	if err == nil {
		t.Fatal("expected error on malformed model output")
	}
}

func TestOllamaRejectsInvalidResponses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"error status despite valid body", 500, `{"message":{"content":"{}"}}`},
		{"not an object", 200, `{"message":{"content":"null"}}`},
		{"oversized", 200, `{"message":{"content":"{}"}}` + strings.Repeat(" ", (1<<20)+1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			if _, err := NewOllamaExtractor(server.URL, "fixture").Extract(context.Background(), "synthetic"); err == nil {
				t.Fatal("accepted invalid response")
			}
		})
	}
}
