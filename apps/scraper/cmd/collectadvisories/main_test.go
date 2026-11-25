package main

import "testing"

func TestPaginationNeverLeavesPublicAdvisoryEndpoint(t *testing.T) {
	valid := `<https://api.github.com/advisories?after=abc&per_page=100>; rel="next"`
	got, err := nextPage(valid)
	if err != nil || got != "https://api.github.com/advisories?after=abc&per_page=100" {
		t.Fatalf("valid link: %s %v", got, err)
	}
	for _, v := range []string{`<http://api.github.com/advisories>; rel="next"`, `<https://evil.example/advisories>; rel="next"`, `<https://api.github.com/user>; rel="next"`, `broken; rel="next"`} {
		if _, err := nextPage(v); err == nil {
			t.Fatal("untrusted pagination accepted")
		}
	}
	if got, err := nextPage(""); err != nil || got != "" {
		t.Fatal("end of pagination not respected")
	}
}
