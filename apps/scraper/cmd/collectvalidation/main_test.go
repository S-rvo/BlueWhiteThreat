package main

import (
	"strings"
	"testing"
)

func TestCanonicalThreadAndHostBoundary(t *testing.T) {
	s := source{Name: "developpez", Language: "fr", Kind: "forum_post", Stratum: "fr_forum"}
	body := "<p>" + strings.Repeat("Discussion technique sur les paramètres du système et du réseau. ", 4) + "</p>"
	d, err := makeDocument(s, "Discussion", body, "http://www.developpez.net/forums/d12345/test/", "now")
	if err != nil || d == nil {
		t.Fatalf("expected extracted discussion: %v", err)
	}
	if d.ID != "developpez:12345" || d.URL != "https://www.developpez.net/forums/showthread.php?t=12345" {
		t.Fatal("unstable thread grouping")
	}
	if _, err = makeDocument(s, "Discussion", body, "https://www.developpez.net.evil.example/forums/d12345/", "now"); err == nil {
		t.Fatal("accepted unexpected host")
	}
}
func TestShortOrBlockedDocumentNeverRetained(t *testing.T) {
	s := source{Name: "security", Language: "en", Kind: "question", Stratum: "en_security"}
	for _, body := range []string{"<p>short</p>", "<p>" + strings.Repeat("CSAM-filter-test-marker ", 20) + "</p>"} {
		d, err := makeDocument(s, "Question", body, "https://security.stackexchange.com/questions/1", "now")
		if err != nil || d != nil {
			t.Fatalf("unsafe/short document retained: %v", err)
		}
	}
}

func TestStackQuestionsURLUsesDisjointCreationWindow(t *testing.T) {
	got := stackQuestionsURL("security", 1, 1700983370)
	if !strings.Contains(got, "site=security") || !strings.Contains(got, "page=1") || !strings.Contains(got, "sort=creation") || !strings.Contains(got, "max=1700983370") {
		t.Fatalf("unexpected bounded Stack Exchange URL: %s", got)
	}
	if strings.Contains(stackQuestionsURL("security", 1, 0), "max=") {
		t.Fatal("default Stack Exchange URL unexpectedly constrained")
	}
}

func TestStackExchangeCreationDatePersistsInUTC(t *testing.T) {
	if got := formatSourceDate(1700000000); got != "2023-11-14T22:13:20Z" {
		t.Fatalf("unexpected UTC source date: %s", got)
	}
	if got := formatSourceDate(0); got != "" {
		t.Fatalf("missing source date should stay empty, got %q", got)
	}
}
