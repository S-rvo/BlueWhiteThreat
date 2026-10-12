// apps/classifier/internal/bootstrap/label.go
package bootstrap

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// MinKeywordHits is the number of distinct seed keywords that must appear
// in a document for the bootstrap pass to provisionally label it relevant.
// This is a cheap, noisy heuristic meant to be corrected by a human before
// training — see docs/plans/2026-10-01-relevance-classifier-design.md.
const MinKeywordHits = 3

// maxOnionMentions caps how many ".onion" substrings a document can contain
// before it's treated as a link directory (hidden wiki, market index, etc.)
// rather than actual content. These pages trivially trip the keyword
// heuristic because they list category names ("hacking", "leaked", "drugs")
// as navigation labels, not because they describe real CTI-relevant events —
// training on them as positives was the dominant source of label noise.
const maxOnionMentions = 9

// directoryMarkers are phrases that only appear on link-directory/hidden-wiki
// navigation pages, never in substantive content — catches directory pages
// with few .onion mentions that the onion-count threshold alone would miss
// (e.g. "OnionDir", "TorLinks", "Hidden Wiki" index pages).
var directoryMarkers = []string{
	"add link",
	"add site",
	"onion directory",
	"link directory",
	"hidden wiki",
	"onion links",
	"onionlinks",
	"onion list",
	"onion dir",
	"tor links",
	"torlinks",
}

// IsLinkDirectory reports whether text looks like a directory/index page of
// onion links rather than substantive content.
func IsLinkDirectory(text string) bool {
	lower := strings.ToLower(text)
	if strings.Count(lower, ".onion") > maxOnionMentions {
		return true
	}
	for _, marker := range directoryMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// LabelByKeywords returns 1 (relevant) if text contains at least
// MinKeywordHits of the given keywords, 0 otherwise.
func LabelByKeywords(text string, keywords []string) int {
	lower := strings.ToLower(text)
	hits := 0
	for _, kw := range keywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			hits++
		}
	}
	if hits >= MinKeywordHits {
		return 1
	}
	return 0
}

// LoadKeywords reads the seed keyword list from a JSON file (array of strings).
func LoadKeywords(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: read keywords file %s: %w", path, err)
	}
	var keywords []string
	if err := json.Unmarshal(data, &keywords); err != nil {
		return nil, fmt.Errorf("bootstrap: unmarshal keywords %s: %w", path, err)
	}
	return keywords, nil
}
