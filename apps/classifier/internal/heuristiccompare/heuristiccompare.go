// apps/classifier/internal/heuristiccompare/heuristiccompare.go
//
// Reproductions of the OLD (pre-rewrite) and a parameterized NEW
// keyword-counting heuristic, used only by comparison tooling
// (cmd/compareheuristics, cmd/evalheuristics, cmd/comparecandidates) to
// isolate the effect of Codex's longest-non-overlapping-phrase rewrite
// (internal/bootstrap.LabelByKeywords) from unrelated changes (e.g. the
// frozen train/val/test split). Neither function is used by production
// inference or training.
package heuristiccompare

import "strings"

// OldLabelByKeywords reproduces the pre-rewrite heuristic: a simple,
// case-insensitive substring count of distinct keywords, positive if at
// least one distinct keyword occurs anywhere in the text. Unlike the current
// internal/bootstrap.LabelByKeywords, it does not tokenize on word
// boundaries, does not prefer the longest overlapping phrase, and has no
// minimum-hit-count floor.
func OldLabelByKeywords(doc string, keywords []string) int {
	lower := strings.ToLower(doc)
	for _, kw := range keywords {
		if kw == "" {
			continue
		}
		if strings.Contains(lower, strings.ToLower(kw)) {
			return 1
		}
	}
	return 0
}

// NewLabelByKeywordsWithMin reproduces internal/bootstrap.LabelByKeywords
// but with the minimum distinct-keyword-hit count as a parameter instead of
// the fixed bootstrap.MinKeywordHits constant, so comparison tools can sweep
// it on validation without touching bootstrap's shipping behavior.
func NewLabelByKeywordsWithMin(doc string, keywords []string, minMatches int) int {
	if countDistinctKeywordHits(doc, keywords) >= minMatches {
		return 1
	}
	return 0
}

func countDistinctKeywordHits(doc string, keywords []string) int {
	lower := strings.ToLower(doc)
	hits := 0
	seen := make(map[string]bool)
	for _, kw := range keywords {
		key := strings.ToLower(kw)
		if key == "" || seen[key] {
			continue
		}
		if strings.Contains(lower, key) {
			seen[key] = true
			hits++
		}
	}
	return hits
}
