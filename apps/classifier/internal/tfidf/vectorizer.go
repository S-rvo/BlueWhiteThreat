// apps/classifier/internal/tfidf/vectorizer.go
package tfidf

import (
	"math"
	"sort"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/text"
)

// MaxVocabularySize caps the number of distinct tokens retained in a fitted
// Vectorizer's vocabulary, bounding model size.
const MaxVocabularySize = 5000

// Vectorizer converts documents into fixed-length TF-IDF vectors using a
// frozen vocabulary and set of IDF weights. A Vectorizer can be built via
// Fit, or constructed directly (e.g. when loading vocabulary/IDF data
// deserialized from model.json at inference time).
type Vectorizer struct {
	Vocabulary map[string]int // token -> index in the TF-IDF vector
	IDF        []float64      // idf weight per vocabulary index
}

// Fit builds the vocabulary and IDF weights from a training corpus.
//
// Vocabulary is capped at MaxVocabularySize, keeping the tokens with the
// highest document frequency (most common across documents) — this bounds
// model size while keeping the most broadly discriminative terms. Ties in
// document frequency are broken by alphabetical token ordering, and
// vocabulary indices are assigned in that same deterministic order
// (descending document frequency, then alphabetical), so Fit produces
// identical output across runs regardless of Go's randomized map iteration
// order.
//
// IDF per retained token = log((n+1)/(df+1)) + 1, where n = number of
// documents and df = document frequency of the token (smoothed to avoid
// division by zero and to avoid a zero IDF for terms in every document).
func Fit(documents []string) *Vectorizer {
	docFreq := make(map[string]int)
	for _, doc := range documents {
		seen := make(map[string]struct{})
		for _, tok := range text.Tokenize(doc) {
			seen[tok] = struct{}{}
		}
		for tok := range seen {
			docFreq[tok]++
		}
	}

	tokens := make([]string, 0, len(docFreq))
	for tok := range docFreq {
		tokens = append(tokens, tok)
	}
	sort.Slice(tokens, func(i, j int) bool {
		if docFreq[tokens[i]] != docFreq[tokens[j]] {
			return docFreq[tokens[i]] > docFreq[tokens[j]]
		}
		return tokens[i] < tokens[j]
	})

	if len(tokens) > MaxVocabularySize {
		tokens = tokens[:MaxVocabularySize]
	}

	n := float64(len(documents))
	vocabulary := make(map[string]int, len(tokens))
	idf := make([]float64, len(tokens))
	for i, tok := range tokens {
		vocabulary[tok] = i
		df := float64(docFreq[tok])
		idf[i] = math.Log((n+1)/(df+1)) + 1
	}

	return &Vectorizer{Vocabulary: vocabulary, IDF: idf}
}

// Transform converts a single document into a TF-IDF vector of length
// len(Vocabulary), using this Vectorizer's frozen vocabulary and IDF.
// TF is raw term frequency within the document (count of token occurrences).
// Each vector entry is TF(token) * IDF(token) for tokens in the vocabulary;
// tokens not in the vocabulary are ignored. Returns a zero vector (all 0.0,
// length len(Vocabulary)) for an empty document or a document containing
// only unknown (out-of-vocabulary) tokens.
func (v *Vectorizer) Transform(document string) []float64 {
	vec := make([]float64, len(v.Vocabulary))
	for _, tok := range text.Tokenize(document) {
		idx, ok := v.Vocabulary[tok]
		if !ok {
			continue
		}
		vec[idx] += v.IDF[idx]
	}
	return vec
}
