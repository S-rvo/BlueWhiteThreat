// apps/classifier/internal/dedup/dedup.go
package dedup

import (
	"regexp"
	"sort"
	"strings"
)

var whitespaceRun = regexp.MustCompile(`\s+`)

// Normalize collapses a document to a canonical form used to detect
// duplicates: lowercased, with runs of whitespace collapsed to a single
// space, and leading/trailing whitespace trimmed. Two scrapes of the same
// page that differ only in casing or incidental whitespace normalize to the
// same key. This does not catch paraphrased near-duplicates (fuzzy/edit-
// distance matching), only exact-after-normalization ones.
func Normalize(text string) string {
	return strings.TrimSpace(whitespaceRun.ReplaceAllString(strings.ToLower(text), " "))
}

// Group is a set of documents that normalize to the same text, assumed to
// be the same underlying content (e.g. scraped twice), with a single label
// shared by every member. Group only exists for label-consistent
// duplicates — label-conflicting duplicates are reported as a Conflict
// instead (see GroupByNormalizedText).
type Group struct {
	Key   string
	IDs   []string
	Texts []string
	Label float64
}

// Conflict is a set of documents that normalize to the same text but
// disagree on label — the data itself is contradictory, so callers must
// surface this rather than silently picking one label.
type Conflict struct {
	Key    string
	IDs    []string
	Texts  []string
	Labels []float64
}

// GroupByNormalizedText groups ids/texts/labels (parallel slices, same
// length) by Normalize(text). Groups whose members all share one label are
// returned in groups, ordered deterministically by normalized key; groups
// whose members disagree on label are returned in conflicts instead and are
// excluded from groups entirely.
func GroupByNormalizedText(ids, texts []string, labels []float64) (groups []Group, conflicts []Conflict) {
	type bucket struct {
		ids    []string
		texts  []string
		labels []float64
	}
	buckets := make(map[string]*bucket)
	var keys []string
	for i, text := range texts {
		key := Normalize(text)
		b, ok := buckets[key]
		if !ok {
			b = &bucket{}
			buckets[key] = b
			keys = append(keys, key)
		}
		b.ids = append(b.ids, ids[i])
		b.texts = append(b.texts, texts[i])
		b.labels = append(b.labels, labels[i])
	}
	sort.Strings(keys) // deterministic regardless of map iteration order

	for _, key := range keys {
		b := buckets[key]
		consistent := true
		for _, l := range b.labels {
			if l != b.labels[0] {
				consistent = false
				break
			}
		}
		if consistent {
			groups = append(groups, Group{Key: key, IDs: b.ids, Texts: b.texts, Label: b.labels[0]})
		} else {
			conflicts = append(conflicts, Conflict{Key: key, IDs: b.ids, Texts: b.texts, Labels: b.labels})
		}
	}
	return groups, conflicts
}
