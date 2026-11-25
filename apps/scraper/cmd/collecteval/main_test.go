package main

import (
	"strings"
	"testing"
)

func TestEligibleUsesMainContentAndRejectsBlockedText(t *testing.T) {
	q := question{ID: 1, Title: "Display troubleshooting", Link: "https://superuser.com/questions/1/example", Body: "<p>" + strings.Repeat("ordinary display troubleshooting ", 8) + "</p><script>invisible script</script>"}
	d, err := eligible(q, "superuser", "en", "2026-10-19T00:00:00Z")
	if err != nil || d == nil {
		t.Fatalf("eligible: %v", err)
	}
	if strings.Contains(d.Text, "invisible") || d.SourceType != "public_qa_pilot" || d.Group != d.ID {
		t.Fatal("extraction/provenance mismatch")
	}
	q.Body = "<p>Synthetic safety marker: CSAM</p>" + q.Body
	if d, err = eligible(q, "superuser", "en", ""); err != nil || d != nil {
		t.Fatal("blocked content retained")
	}
	q.Body = strings.Repeat("ordinary text ", 20)
	q.Link = "https://example.org/questions/1"
	if _, err = eligible(q, "superuser", "en", ""); err == nil {
		t.Fatal("noncanonical source accepted")
	}
}
