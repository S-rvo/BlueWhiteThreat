// Package csamfilter is the single source of truth for CSAM-indicating
// vocabulary and known offending hosts, shared by the crawler (URL gate,
// checked before ever requesting or queuing a page) and the scraper
// (content gate, checked before persisting a scraped page). Having one
// canonical list avoids the two apps drifting out of sync as new evasion
// patterns are discovered.
package csamfilter

import "strings"

// vocabulary is the canonical list of CSAM-indicative phrases/fragments. It
// is checked against both free-form page text (space-separated) and URLs
// (hyphen-separated host/path segments) via normalizeVariants, so a single
// list covers both surfaces without needing separate maintenance.
var vocabulary = []string{
	"child porn", "childporn", "child porno", "childporno",
	"pthc", "lolita", "loli porn", "loliporn", "loli",
	"jailbait", "csam", "cp links", "hard cp",
	"pedo", "pedomom", "kidsparty", "pedo porn",
	"preteen", "pre-teen", "underage",
	"diamond porn links",
	"incest", "insest", "toddler", "nymphet",
	"little boy", "little girl",
	// Discovered 2026-10-08 via the crawler's queue actively discovering
	// dozens of hash variants of these patterns, none yet scraped.
	"teen boy porn", "punished young",
	"family love free vidios", "family love free",
	// "chuby" added 2026-10-11: same spam farm, 11 distinct hash variants
	// seen across the two most recent batches alone.
	"chuby",
	// Added 2026-10-18: a farm mints a brand new .onion hash and
	// subdomain-style prefix for nearly every URL, defeating both the hash
	// blocklist and the dynamic hash-reuse detector (farmdetect.go) since
	// each hash is only ever seen once. But it reuses the same small
	// library of CSAM-indicative path segments across all of them (e.g.
	// "/leaked-snapchat-preteens/...", "/preteen-vibrator/..."), and
	// deliberately embeds cyber-threat-intel keywords like "leak" into
	// those paths to collide with the crawler's topical-relevance filter.
	// Matching on this vocabulary closes that gap regardless of host/hash.
	"kid porn", "kids porn",
	"young girl porn", "boy porn", "girl sex young",
	"teen fuck", "teen porn",
	"boy fuck", "girl fuck",
	"young girl", "young boy",
	"cp site", "cp link",
}

// urlOnlyPatterns are fragments meaningful only as raw URL substrings (host
// or path), too generic to run against free-form page text without
// producing false positives (e.g. "-cp-" would match unrelated abbreviations
// in prose).
var urlOnlyPatterns = []string{
	"-cp-",
}

// normalizeVariants returns term plus its hyphen/space-swapped form, so a
// single canonical entry matches both "little boy" (free text) and
// "little-boy" (URL path segment).
func normalizeVariants(term string) []string {
	variants := []string{term}
	if strings.Contains(term, " ") {
		variants = append(variants, strings.ReplaceAll(term, " ", "-"))
	}
	if strings.Contains(term, "-") {
		variants = append(variants, strings.ReplaceAll(term, "-", " "))
	}
	return variants
}

func containsAnyVariant(low string, terms []string) bool {
	for _, term := range terms {
		for _, variant := range normalizeVariants(term) {
			if strings.Contains(low, variant) {
				return true
			}
		}
	}
	return false
}

// ContainsBlockedContent reports whether free-form text (e.g. a scraped
// page body) matches known CSAM-indicating vocabulary.
func ContainsBlockedContent(content string) bool {
	return containsAnyVariant(strings.ToLower(content), vocabulary)
}

// IsBlockedURL reports whether a URL (host or path) is a known CSAM-indexing
// site, a known farm backend hash, or matches known CSAM-indicative
// vocabulary. Exported so the crawler can skip these URLs before even
// issuing the HTTP request, not just before queuing them in Redis.
func IsBlockedURL(rawURL string) bool {
	low := strings.ToLower(rawURL)

	for host := range blockedHosts {
		if strings.Contains(low, host) {
			return true
		}
	}
	for _, hash := range blockedBackendHashes {
		if strings.Contains(low, hash) {
			return true
		}
	}
	if containsAnyVariant(low, vocabulary) {
		return true
	}
	for _, pattern := range urlOnlyPatterns {
		if strings.Contains(low, pattern) {
			return true
		}
	}
	return false
}
