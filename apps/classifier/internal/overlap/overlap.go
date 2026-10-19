// apps/classifier/internal/overlap/overlap.go
//
// Near-duplicate detection used by the dedup/grouping tools (cmd/cvclassweights,
// cmd/cvocabulary, cmd/comparehumanl2, cmd/auditannotationgroups,
// cmd/compareassistant, cmd/comparevocabulary) to find documents that are
// textually near-identical even when not byte-for-byte equal: normalized
// exact match, or five-word-shingle Jaccard similarity >= 0.80 with both
// documents having at least 20 tokens.
package overlap

import (
	"strings"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/text"
)

// shingleSize is the number of adjacent tokens per shingle used for the
// Jaccard similarity comparison.
const shingleSize = 5

// minTokensForShingling is the minimum token count a document must have
// before shingle-based similarity applies; shorter documents are only
// matched via the normalized-exact-text shortcut.
const minTokensForShingling = 20

// jaccardThreshold is the minimum shingle-set Jaccard similarity for two
// sufficiently long documents to be considered near-duplicates.
const jaccardThreshold = 0.80

// Print is a precomputed fingerprint of a document, suitable for cheap
// pairwise comparison via Similar.
type Print struct {
	normalized string
	tokens     int
	shingles   map[string]struct{}
}

// Fingerprint builds a Print for text, tokenizing it and indexing its
// five-word shingles.
func Fingerprint(doc string) Print {
	tokens := text.Tokenize(doc)
	shingles := make(map[string]struct{})
	for i := 0; i+shingleSize <= len(tokens); i++ {
		shingles[strings.Join(tokens[i:i+shingleSize], " ")] = struct{}{}
	}
	return Print{
		normalized: strings.Join(tokens, " "),
		tokens:     len(tokens),
		shingles:   shingles,
	}
}

// Similar reports whether a and b are near-duplicates: identical normalized
// text, or both at least minTokensForShingling tokens long with a
// five-word-shingle Jaccard similarity of at least jaccardThreshold.
func Similar(a, b Print) bool {
	if a.normalized == b.normalized {
		return true
	}
	if a.tokens < minTokensForShingling || b.tokens < minTokensForShingling {
		return false
	}
	if len(a.shingles) == 0 || len(b.shingles) == 0 {
		return false
	}

	intersection := 0
	for s := range a.shingles {
		if _, ok := b.shingles[s]; ok {
			intersection++
		}
	}
	union := len(a.shingles) + len(b.shingles) - intersection
	if union == 0 {
		return false
	}
	return float64(intersection)/float64(union) >= jaccardThreshold
}
