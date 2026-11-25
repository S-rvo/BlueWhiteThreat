// apps/classifier/internal/tfidf/structural.go
package tfidf

import "regexp"

// structuralIndicator pairs a synthetic vocabulary token with a regex that
// detects it in raw document text.
type structuralIndicator struct {
	token   string
	pattern *regexp.Regexp
}

// structuralIndicators are CTI-specific structural signals that a pure
// word/n-gram vocabulary can miss: a decisive marker (a CVE ID, a crypto
// wallet, an onion address, a price) can be rare enough that chi-square
// ranking crowds it out of the capped vocabulary even though its presence
// is highly label-informative. Each match is emitted as a synthetic token
// so it flows through Fit's existing document-frequency/chi-square ranking
// and Transform's existing log-TF*IDF weighting exactly like any word
// token, without a separate feature-vector code path.
var structuralIndicators = []structuralIndicator{
	{"s_onion_link", regexp.MustCompile(`(?i)[a-z2-7]{16,56}\.onion\b`)},
	{"s_btc_wallet", regexp.MustCompile(`\b(bc1[a-zA-HJ-NP-Z0-9]{25,39}|[13][a-zA-HJ-NP-Z0-9]{25,34})\b`)},
	{"s_xmr_wallet", regexp.MustCompile(`\b4[0-9AB][1-9A-HJ-NP-Za-km-z]{93}\b`)},
	{"s_cve_mention", regexp.MustCompile(`(?i)\bcve-\d{4}-\d{1,7}\b`)},
	{"s_price_mention", regexp.MustCompile(`(?i)(\$\s?\d+(\.\d+)?|\b\d+(\.\d+)?\s?(btc|xmr|usd|eur)\b)`)},
	{"s_marketplace_boilerplate", regexp.MustCompile(`(?i)\b(add to cart|in stock|vendor|store:|escrow protected|product category)\b`)},
	{"s_corporate_boilerplate", regexp.MustCompile(`(?i)\b(about us|contact us|privacy policy|terms of service|all rights reserved)\b`)},
	{"s_hosting_boilerplate", regexp.MustCompile(`(?i)\b(web hosting|domain registration|dedicated server|customer support)\b`)},
	{"s_financial_fraud_market", regexp.MustCompile(`(?i)\b(cloned? (credit |debit )?cards?|prepaid (visa |master ?card|cards?)|counterfeit (money|currency)|western union transfer|paypal transfer)\b`)},
	{"s_gambling_market", regexp.MustCompile(`(?i)\b(casino|sportsbook|betting|slots|jackpot)\b`)},
	{"s_darkweb_directory", regexp.MustCompile(`(?i)\b(onion directory|darkweb links?|deep web links?|add new site|search engines?)\b`)},
	{"s_compromise_evidence", regexp.MustCompile(`(?i)\b(stolen data|leaked data|initial access|access broker|ransom demand|data exfiltrat(?:ed|ion)|network breached)\b`)},
}

// structuralTokens scans doc for every structuralIndicators pattern and
// returns one occurrence of the indicator's token per match, so a document
// mentioning two CVEs contributes "s_cve_mention" twice.
func structuralTokens(doc string) []string {
	var out []string
	present := make(map[string]bool)
	for _, ind := range structuralIndicators {
		matches := ind.pattern.FindAllString(doc, -1)
		present[ind.token] = len(matches) > 0
		for range matches {
			out = append(out, ind.token)
		}
	}
	// Conjunctions let the linear head distinguish generic page templates from
	// the same templates accompanied by explicit threat evidence.
	for _, pageType := range []string{"s_marketplace_boilerplate", "s_corporate_boilerplate", "s_hosting_boilerplate", "s_financial_fraud_market", "s_gambling_market", "s_darkweb_directory"} {
		if !present[pageType] {
			continue
		}
		if present["s_compromise_evidence"] {
			out = append(out, pageType+"_with_evidence")
		} else {
			out = append(out, pageType+"_without_evidence")
		}
	}
	// A price is common in both ordinary commerce and extortion claims. Keep
	// the legacy price token for backward-compatible models, and add explicit
	// context interactions so a newly trained model can learn that price alone
	// is not evidence of CTI while price tied to an explicit compromise claim
	// may be relevant. Existing frozen models ignore these new features unless
	// they contain the corresponding vocabulary entries.
	if present["s_price_mention"] {
		if present["s_compromise_evidence"] {
			out = append(out, "s_price_mention_with_evidence")
		} else {
			out = append(out, "s_price_mention_without_evidence")
		}
	}
	return out
}
