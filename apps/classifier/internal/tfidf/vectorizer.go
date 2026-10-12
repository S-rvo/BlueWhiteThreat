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

// charNgramSize is the window length for character n-gram features.
const charNgramSize = 4

// charNgramPrefix distinguishes character n-gram tokens ("c_rans") from
// word tokens, so a short word that happens to equal a 4-gram never
// collides with it in the vocabulary.
const charNgramPrefix = "c_"

// charNgrams returns every overlapping charNgramSize-rune window of token,
// prefixed to its own namespace. Dark-web content is full of l33tspeak and
// typo variants of the same word ("ransomw4re", "r4nsomware") that share
// almost no word tokens but share most character n-grams, so this gives
// the model a feature that still picks up the family resemblance where a
// pure word-token vocabulary would see unrelated tokens. Tokens shorter
// than charNgramSize produce no n-grams; the word token itself already
// covers that case.
func charNgrams(token string) []string {
	runes := []rune(token)
	if len(runes) < charNgramSize {
		return nil
	}
	grams := make([]string, 0, len(runes)-charNgramSize+1)
	for i := 0; i+charNgramSize <= len(runes); i++ {
		grams = append(grams, charNgramPrefix+string(runes[i:i+charNgramSize]))
	}
	return grams
}

// terms splits doc into unigrams (via text.Tokenize), adjacent-pair
// bigrams (so fixed phrases like "whatsapp hacker" get their own
// vocabulary entry distinct from their component words), character
// n-grams of each unigram (see charNgrams), and structural indicator
// tokens (see structuralTokens) matched against the raw, untokenized doc
// text so patterns like CVE IDs or onion addresses aren't broken apart by
// tokenization first.
func terms(doc string) []string {
	unigrams := text.Tokenize(doc)
	out := make([]string, 0, 2*len(unigrams))
	out = append(out, unigrams...)
	for i := 0; i+1 < len(unigrams); i++ {
		out = append(out, unigrams[i]+"_"+unigrams[i+1])
	}
	for _, tok := range unigrams {
		out = append(out, charNgrams(tok)...)
	}
	out = append(out, structuralTokens(doc)...)
	return out
}

// Fit builds the vocabulary and IDF weights from a training corpus. labels
// must be the same length as documents (0 = not relevant, 1 = relevant).
//
// Vocabulary is capped at MaxVocabularySize. Candidate tokens are ranked by
// chi-square association with the label — a token that skews strongly
// towards one class outranks a token that's merely common — so a rare but
// decisive term (e.g. a specific scam-service brand name) isn't crowded out
// by frequent, label-agnostic boilerplate. Tokens with no label association
// (chi-square 0, including the degenerate case of a single-class corpus)
// fall back to being ranked by document frequency, then alphabetically, so
// Fit's behavior degrades gracefully to plain document-frequency ranking
// when the label signal is absent. Vocabulary indices are assigned in that
// same deterministic order, so Fit produces identical output across runs
// regardless of Go's randomized map iteration order.
//
// IDF per retained token = log((n+1)/(df+1)) + 1, where n = number of
// documents and df = document frequency of the token (smoothed to avoid
// division by zero and to avoid a zero IDF for terms in every document).
func Fit(documents []string, labels []float64) *Vectorizer {
	docFreq := make(map[string]int)
	posFreq := make(map[string]int) // docFreq restricted to label==1 docs
	nPos := 0
	for i, doc := range documents {
		isPos := labels[i] == 1
		if isPos {
			nPos++
		}
		seen := make(map[string]struct{})
		for _, tok := range terms(doc) {
			seen[tok] = struct{}{}
		}
		for tok := range seen {
			docFreq[tok]++
			if isPos {
				posFreq[tok]++
			}
		}
	}
	nNeg := len(documents) - nPos

	tokens := make([]string, 0, len(docFreq))
	for tok := range docFreq {
		tokens = append(tokens, tok)
	}

	chi2 := make(map[string]float64, len(tokens))
	for _, tok := range tokens {
		a := float64(posFreq[tok])         // has token, positive
		b := float64(docFreq[tok] - posFreq[tok]) // has token, negative
		c := float64(nPos) - a              // lacks token, positive
		d := float64(nNeg) - b              // lacks token, negative
		denom := (a + b) * (c + d) * (a + c) * (b + d)
		if denom == 0 {
			chi2[tok] = 0
			continue
		}
		diff := a*d - b*c
		chi2[tok] = float64(len(documents)) * diff * diff / denom
	}

	sort.Slice(tokens, func(i, j int) bool {
		if chi2[tokens[i]] != chi2[tokens[j]] {
			return chi2[tokens[i]] > chi2[tokens[j]]
		}
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
// TF is log-scaled term frequency within the document: log1p(count) of
// occurrences of a token (unigrams, adjacent-pair bigrams, and character
// n-grams alike), so a term repeated many times (e.g. a spammed keyword,
// or a generic word naturally common in long dark-web listings) doesn't
// contribute linearly more signal than the same term appearing once or
// twice — it dampens raw repetition while still rewarding presence growing
// weight. Each vector entry is logTF(token) * IDF(token) for tokens in the
// vocabulary; tokens not in the vocabulary are ignored. The resulting
// vector is L2-normalized so a document's score isn't a function of its
// raw length — otherwise a long page repeating generic boilerplate (e.g. a
// crypto-rate ticker) outscores a short, information-dense one on term
// volume alone. Returns a zero vector (all 0.0, length len(Vocabulary)) for
// an empty document or a document containing only unknown (out-of-vocabulary)
// tokens.
func (v *Vectorizer) Transform(document string) []float64 {
	counts := make(map[int]int)
	for _, tok := range terms(document) {
		idx, ok := v.Vocabulary[tok]
		if !ok {
			continue
		}
		counts[idx]++
	}

	vec := make([]float64, len(v.Vocabulary))
	for idx, count := range counts {
		vec[idx] = math.Log1p(float64(count)) * v.IDF[idx]
	}

	var sumSq float64
	for _, val := range vec {
		sumSq += val * val
	}
	if sumSq > 0 {
		norm := math.Sqrt(sumSq)
		for i := range vec {
			vec[i] /= norm
		}
	}
	return vec
}
