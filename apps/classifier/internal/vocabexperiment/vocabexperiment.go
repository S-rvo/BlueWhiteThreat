// apps/classifier/internal/vocabexperiment/vocabexperiment.go
//
// Word-only (no character n-gram) vocabulary fitting, used only by
// cmd/comparevocabulary and cmd/cvocabulary to test whether
// internal/tfidf's character n-gram features actually help over a plain
// word/bigram vocabulary of the same cap. Returns a *tfidf.Vectorizer so
// Transform still runs tfidf's own term extraction (unigrams, bigrams,
// character n-grams, structural tokens) — but since the returned
// Vectorizer's vocabulary never contains character n-gram or structural
// tokens, those terms are simply ignored at transform time as
// out-of-vocabulary, leaving an effectively word-only (optionally
// word+bigram) feature set.
package vocabexperiment

import (
	"math"
	"sort"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/text"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/tfidf"
)

func wordTerms(doc string, bigrams bool) []string {
	unigrams := text.Tokenize(doc)
	if !bigrams {
		return unigrams
	}
	out := make([]string, 0, 2*len(unigrams))
	out = append(out, unigrams...)
	for i := 0; i+1 < len(unigrams); i++ {
		out = append(out, unigrams[i]+"_"+unigrams[i+1])
	}
	return out
}

// Fit builds a word-only (or word+bigram, if bigrams is true) vocabulary
// from documents, ranked by document frequency and capped at
// tfidf.MaxVocabularySize, with no minimum document frequency filter.
func Fit(documents []string, bigrams bool) *tfidf.Vectorizer {
	return fit(documents, bigrams, 1)
}

// FitMinDF behaves like Fit but discards any term occurring in fewer than
// minDF documents before ranking and capping.
func FitMinDF(documents []string, bigrams bool, minDF int) *tfidf.Vectorizer {
	if minDF < 1 {
		minDF = 1
	}
	return fit(documents, bigrams, minDF)
}

func fit(documents []string, bigrams bool, minDF int) *tfidf.Vectorizer {
	docFreq := make(map[string]int)
	for _, doc := range documents {
		seen := make(map[string]struct{})
		for _, tok := range wordTerms(doc, bigrams) {
			seen[tok] = struct{}{}
		}
		for tok := range seen {
			docFreq[tok]++
		}
	}

	tokens := make([]string, 0, len(docFreq))
	for tok, df := range docFreq {
		if df >= minDF {
			tokens = append(tokens, tok)
		}
	}

	sort.Slice(tokens, func(i, j int) bool {
		if docFreq[tokens[i]] != docFreq[tokens[j]] {
			return docFreq[tokens[i]] > docFreq[tokens[j]]
		}
		return tokens[i] < tokens[j]
	})

	if len(tokens) > tfidf.MaxVocabularySize {
		tokens = tokens[:tfidf.MaxVocabularySize]
	}

	n := float64(len(documents))
	vocabulary := make(map[string]int, len(tokens))
	idf := make([]float64, len(tokens))
	for i, tok := range tokens {
		vocabulary[tok] = i
		df := float64(docFreq[tok])
		idf[i] = math.Log((n+1)/(df+1)) + 1
	}

	return &tfidf.Vectorizer{Vocabulary: vocabulary, IDF: idf}
}
