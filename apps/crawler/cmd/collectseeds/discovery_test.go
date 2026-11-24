package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExtractSeeds(t *testing.T) {
	host := strings.Repeat("a", 56) + ".onion"
	for _, tc := range []struct{ name, input, want string }{
		{"https path", `https://` + host + `/Forum?id=12&amp;page=2#reply`, "https://" + host + "/Forum?id=12&page=2"},
		{"bare", host, "http://" + host + "/"},
		{"markdown", "[forum](https://" + host + "/Forum)", "https://" + host + "/Forum"},
		{"json", `https:\/\/` + host + `\/Forum`, "https://" + host + "/Forum"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := map[string]bool{}
			extractSeeds(tc.input, got)
			if len(got) != 1 || !got[tc.want] {
				t.Fatalf("got %v want %s", got, tc.want)
			}
		})
	}
}
func TestRejectLegacyMentions(t *testing.T) {
	got := map[string]bool{}
	extractSeeds(strings.Repeat("a", 16)+".onion", got)
	if len(got) != 0 {
		t.Fatal(got)
	}
}
func TestRejectOversizedSource(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(strings.Repeat("x", maxBodyBytes+1))) }))
	defer s.Close()
	if _, err := fetchOnionAddresses(s.Client(), s.URL, map[string]bool{}); err == nil {
		t.Fatal("expected size error")
	}
}

func TestSourceLabelOmitsSessionFields(t *testing.T) {
	got := sourceLabel("https://ahmia.fi/search/?q=forum&session=secret")
	if got != "https://ahmia.fi/search/?q=forum" {
		t.Fatal(got)
	}
}
