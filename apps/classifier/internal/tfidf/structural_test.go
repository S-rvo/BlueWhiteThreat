// apps/classifier/internal/tfidf/structural_test.go
package tfidf

import "testing"

func countToken(tokens []string, want string) int {
	n := 0
	for _, tok := range tokens {
		if tok == want {
			n++
		}
	}
	return n
}

func TestStructuralTokens_DetectsOnionLink(t *testing.T) {
	doc := "visit us at abcdefghijklmnopqrstuvwxyz234567abcdefghijklmnop23456.onion for details"
	tokens := structuralTokens(doc)
	if countToken(tokens, "s_onion_link") != 1 {
		t.Fatalf("expected one s_onion_link token, got %v", tokens)
	}
}

func TestStructuralTokens_DetectsCVEMention(t *testing.T) {
	doc := "exploit for CVE-2024-12345 is now for sale, also affects cve-2023-1"
	tokens := structuralTokens(doc)
	if countToken(tokens, "s_cve_mention") != 2 {
		t.Fatalf("expected two s_cve_mention tokens, got %v", tokens)
	}
}

func TestStructuralTokens_DetectsBTCWallet(t *testing.T) {
	doc := "send payment to 1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa to proceed"
	tokens := structuralTokens(doc)
	if countToken(tokens, "s_btc_wallet") != 1 {
		t.Fatalf("expected one s_btc_wallet token, got %v", tokens)
	}
}

func TestStructuralTokens_DetectsPriceMention(t *testing.T) {
	doc := "price is $500 or 0.01 btc, negotiable"
	tokens := structuralTokens(doc)
	if countToken(tokens, "s_price_mention") != 2 {
		t.Fatalf("expected two s_price_mention tokens, got %v", tokens)
	}
}

func TestStructuralTokens_IgnoresOrdinaryText(t *testing.T) {
	doc := "a new ransomware campaign is spreading across multiple organizations"
	tokens := structuralTokens(doc)
	if len(tokens) != 0 {
		t.Fatalf("expected no structural tokens for ordinary text, got %v", tokens)
	}
}

func TestFitIncludesStructuralIndicators(t *testing.T) {
	// A document mentioning a CVE is a strong, label-correlated structural
	// signal distinct from any single word token - Fit must pick it up as
	// a vocabulary entry like any other term, and Transform must score it.
	docs := []string{
		"exploit for CVE-2024-98765 sold here",
		"exploit for CVE-2024-98765 sold here",
		"unrelated quarterly earnings report",
	}
	v := Fit(docs, zeros(len(docs)))

	if _, ok := v.Vocabulary["s_cve_mention"]; !ok {
		t.Fatalf("expected vocabulary to contain %q, vocab=%v", "s_cve_mention", v.Vocabulary)
	}

	vec := v.Transform("CVE-2024-98765")
	idx := v.Vocabulary["s_cve_mention"]
	if vec[idx] <= 0 {
		t.Fatalf("expected Transform to score %q > 0, got %v", "s_cve_mention", vec[idx])
	}
}
