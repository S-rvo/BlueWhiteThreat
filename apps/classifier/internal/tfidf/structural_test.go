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

func TestStructuralTokens_DetectsDarkwebErrorIndicators(t *testing.T) {
	tokens := structuralTokens("Add to cart. Privacy policy. Dedicated server. Stolen data from a network breached last week.")
	for _, want := range []string{"s_marketplace_boilerplate", "s_corporate_boilerplate", "s_hosting_boilerplate", "s_compromise_evidence"} {
		if countToken(tokens, want) < 1 {
			t.Fatalf("expected %s token, got %v", want, tokens)
		}
	}
	if countToken(tokens, "s_marketplace_boilerplate_with_evidence") != 1 || countToken(tokens, "s_corporate_boilerplate_with_evidence") != 1 || countToken(tokens, "s_hosting_boilerplate_with_evidence") != 1 {
		t.Fatalf("expected page-type/evidence interactions, got %v", tokens)
	}
}

func TestStructuralTokens_DistinguishesTemplateWithoutEvidence(t *testing.T) {
	tokens := structuralTokens("Add to cart from this vendor. Contact us and read our privacy policy.")
	if countToken(tokens, "s_marketplace_boilerplate_without_evidence") != 1 || countToken(tokens, "s_corporate_boilerplate_without_evidence") != 1 {
		t.Fatalf("expected without-evidence interactions, got %v", tokens)
	}
}

func TestStructuralTokens_DetectsGenericDarkwebMarketsWithoutEvidence(t *testing.T) {
	tokens := structuralTokens("Buy cloned debit cards. Casino betting and slots. Onion directory and deep web links.")
	for _, want := range []string{"s_financial_fraud_market_without_evidence", "s_gambling_market_without_evidence", "s_darkweb_directory_without_evidence"} {
		if countToken(tokens, want) != 1 {
			t.Fatalf("expected %s, got %v", want, tokens)
		}
	}
}

func TestStructuralTokens_ContextualizesPrices(t *testing.T) {
	ordinary := structuralTokens("Buy a service for 0.5 BTC. Prices are listed in the store.")
	if countToken(ordinary, "s_price_mention") == 0 || countToken(ordinary, "s_price_mention_without_evidence") != 1 {
		t.Fatalf("expected a price-without-evidence interaction, got %v", ordinary)
	}
	incident := structuralTokens("Stolen data identified; ransom demand is 2 BTC.")
	if countToken(incident, "s_price_mention") == 0 || countToken(incident, "s_price_mention_with_evidence") != 1 {
		t.Fatalf("expected a price-with-evidence interaction, got %v", incident)
	}
	noPrice := structuralTokens("Stolen customer records were leaked after a network breach.")
	if countToken(noPrice, "s_price_mention_with_evidence") != 0 || countToken(noPrice, "s_price_mention_without_evidence") != 0 {
		t.Fatalf("price interactions should require a price marker, got %v", noPrice)
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
