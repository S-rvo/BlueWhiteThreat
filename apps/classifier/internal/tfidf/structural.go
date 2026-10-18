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
}

// structuralTokens scans doc for every structuralIndicators pattern and
// returns one occurrence of the indicator's token per match, so a document
// mentioning two CVEs contributes "s_cve_mention" twice.
func structuralTokens(doc string) []string {
	var out []string
	for _, ind := range structuralIndicators {
		for range ind.pattern.FindAllString(doc, -1) {
			out = append(out, ind.token)
		}
	}
	return out
}
