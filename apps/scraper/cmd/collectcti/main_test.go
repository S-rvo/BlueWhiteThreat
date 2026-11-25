package main

import (
	"strings"
	"testing"
)

func TestAllowedArticleRestrictsOfficialCTIPaths(t *testing.T) {
	tests := []struct {
		url, source string
		want        bool
	}{
		{"https://cert.europa.eu/publications/threat-intelligence/cb26-10/", "cert-eu-cti", true},
		{"https://cert.cyber.gouv.fr/cti/CERTFR-2026-CTI-001/", "cert-fr-cti", true},
		{"https://www.cert.cyber.gouv.fr/cti/CERTFR-2023-CTI-003/", "cert-fr-cti", true},
		{"https://www.cisa.gov/news-events/cybersecurity-advisories/aa26-293a", "cisa-advisory", true},
		{"https://isc.sans.edu/diary/rss/33458", "sans-isc-diary", true},
		{"https://isc.sans.edu/diary/33458", "sans-isc-diary", false},
		{"https://evil.example/diary/rss/33458", "sans-isc-diary", false},
		{"https://www.cisa.gov/news-events/other/aa26-293a", "cisa-advisory", false},
		{"https://evil.example/news-events/cybersecurity-advisories/aa26-293a", "cisa-advisory", false},
		{"https://example.org/publications/threat-intelligence/report/", "cert-eu-cti", false},
		{"http://cert.cyber.gouv.fr/cti/report/", "cert-fr-cti", false},
	}
	for _, test := range tests {
		if got := allowedArticle(test.url, test.source); got != test.want {
			t.Fatalf("allowedArticle(%q, %q)=%t, want %t", test.url, test.source, got, test.want)
		}
	}
}

func TestNormalizedDocumentKeepsProvenanceAndHash(t *testing.T) {
	body := []byte("<html><body><article><h1>Campaign</h1><p>" + strings.Repeat("Threat actors used phishing and stolen access. ", 12) + "</p></article></body></html>")
	doc, err := normalizedDocument("cert-eu-cti", "en", "Monthly report", "https://cert.europa.eu/publications/threat-intelligence/example/", body, "2026-10-19T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if doc == nil || doc.Source != "cert-eu-cti" || doc.Group != doc.ID || len(doc.TextSHA256) != 64 {
		t.Fatalf("invalid normalized document: %+v", doc)
	}
	if doc.LabelAuthority != "unscored; source membership is not a label" {
		t.Fatal("source was converted into an automatic label")
	}
}
