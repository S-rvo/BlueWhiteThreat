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
const MinKeywordHits = 2

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
