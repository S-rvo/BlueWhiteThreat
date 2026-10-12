// apps/classifier/internal/text/tokenize.go
package text

import (
	"regexp"
	"strings"
)

var tokenPattern = regexp.MustCompile(`[\p{L}\p{N}]+`)

// Tokenize lowercases s and splits it into unicode-aware word tokens,
// dropping anything shorter than 2 characters (single letters, lone digits)
// since they carry no discriminative signal for TF-IDF.
func Tokenize(s string) []string {
	lower := strings.ToLower(s)
	matches := tokenPattern.FindAllString(lower, -1)
	tokens := make([]string, 0, len(matches))
	for _, m := range matches {
		if len([]rune(m)) < 2 {
			continue
		}
		tokens = append(tokens, m)
	}
	return tokens
}
