// apps/classifier/internal/bootstrap/label.go
package bootstrap

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
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

// Keep Unicode letters and single digits (notably "0-day"). Punctuation
// separates tokens, so spelling variants such as "cve"/"cve-" share a key.
var keywordWords = regexp.MustCompile(`[\p{L}\p{N}_]+`)

// LabelByKeywords counts distinct, non-overlapping keyword phrases. At each
// word, the longest matching phrase wins: "data breach" does not also count
// as "breach". Repeated mentions and equivalent seed spellings count once.
// This remains a lexical bootstrap heuristic, not a semantic relevance test.
func LabelByKeywords(text string, keywords []string) int {
	type phrase struct {
		words []string
		key   string
	}
	byFirst := make(map[string][]phrase)
	unique := make(map[string]bool)
	for _, kw := range keywords {
		words := keywordWords.FindAllString(strings.ToLower(kw), -1)
		if len(words) == 0 {
			continue
		}
		key := strings.Join(words, " ")
		if unique[key] {
			continue
		}
		unique[key] = true
		byFirst[words[0]] = append(byFirst[words[0]], phrase{words, key})
	}

	words := keywordWords.FindAllString(strings.ToLower(text), -1)
	matched := make(map[string]bool)
	for i := 0; i < len(words); {
		var longest phrase
		for _, candidate := range byFirst[words[i]] {
			if len(candidate.words) <= len(longest.words) || i+len(candidate.words) > len(words) {
				continue
			}
			matches := true
			for j, word := range candidate.words {
				if words[i+j] != word {
					matches = false
					break
				}
			}
			if matches {
				longest = candidate
			}
		}
		if len(longest.words) == 0 {
			i++
			continue
		}
		matched[longest.key] = true
		if len(matched) >= MinKeywordHits {
			return 1
		}
		i += len(longest.words)
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
