// apps/classifier/internal/bootstrap/label.go
package bootstrap

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
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

var (
	keywordPatternsMu sync.Mutex
	keywordPatterns   = map[string]*regexp.Regexp{}
)

// keywordPattern compiles (and caches) a word-boundary regex for a keyword.
// A plain substring match would let "ransom" match inside "ransomware",
// double-counting one real signal as two keyword hits. \b only anchors at a
// transition between a word character and a non-word one, so it's only
// added on a side whose edge character is itself a word character —
// otherwise (e.g. a keyword ending in "-", like "cve-") the literal
// character already provides the boundary.
func keywordPattern(kw string) *regexp.Regexp {
	keywordPatternsMu.Lock()
	defer keywordPatternsMu.Unlock()

	if p, ok := keywordPatterns[kw]; ok {
		return p
	}

	isWordChar := func(b byte) bool {
		return b == '_' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
	}

	pattern := regexp.QuoteMeta(kw)
	if isWordChar(kw[0]) {
		pattern = `\b` + pattern
	}
	if isWordChar(kw[len(kw)-1]) {
		pattern = pattern + `\b`
	}

	p := regexp.MustCompile("(?i)" + pattern)
	keywordPatterns[kw] = p
	return p
}

// LabelByKeywords returns 1 (relevant) if text contains at least
// MinKeywordHits of the given keywords, 0 otherwise. Matching is done at
// word boundaries so a keyword only counts when it appears as its own
// token (or its own hyphenated/multi-word phrase), not as a substring of
// an unrelated or more specific word.
func LabelByKeywords(text string, keywords []string) int {
	hits := 0
	for _, kw := range keywords {
		if keywordPattern(kw).MatchString(text) {
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
