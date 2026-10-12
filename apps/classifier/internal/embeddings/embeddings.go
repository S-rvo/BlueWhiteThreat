// apps/classifier/internal/embeddings/embeddings.go
package embeddings

import (
	"math"
	"math/rand"
	"sort"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/text"
)

// BuildWordVectors learns a small distributional embedding for every
// frequent word across docs, entirely in pure Go with no external model,
// no pre-trained table, and no network access: it builds a PPMI (positive
// pointwise mutual information) word-word co-occurrence matrix within a
// sliding window, then projects it down to dim dimensions with a
// fixed-seed random projection. By the Johnson-Lindenstrauss lemma this
// approximately preserves the pairwise cosine similarities a full
// SVD/LSA decomposition would find, at a fraction of the cost, so words
// that occur in similar contexts (near-synonyms, like "malware" and
// "ransomware") end up with similar vectors even though TF-IDF treats
// them as entirely unrelated tokens.
//
// Only the maxVocab most frequent words are embedded, bounding memory and
// runtime on large corpora; rarer words simply have no entry in the
// returned map (callers skip them, as AverageVector and Embedder do).
func BuildWordVectors(docs []string, windowSize, dim, maxVocab int, seed int64) map[string][]float64 {
	if len(docs) == 0 || dim <= 0 {
		return map[string][]float64{}
	}

	tokenized := make([][]string, len(docs))
	docFreq := map[string]int{}
	for i, doc := range docs {
		tokens := text.Tokenize(doc)
		tokenized[i] = tokens
		seen := map[string]bool{}
		for _, tok := range tokens {
			if !seen[tok] {
				docFreq[tok]++
				seen[tok] = true
			}
		}
	}

	vocab := buildVocab(docFreq, maxVocab)
	n := len(vocab)
	if n == 0 {
		return map[string][]float64{}
	}

	coOccur := make([]map[int]float64, n)
	for i := range coOccur {
		coOccur[i] = map[int]float64{}
	}
	totalCount := make([]float64, n)
	var grandTotal float64

	for _, tokens := range tokenized {
		idxs := make([]int, len(tokens))
		for i, tok := range tokens {
			if idx, ok := vocab[tok]; ok {
				idxs[i] = idx
			} else {
				idxs[i] = -1
			}
		}
		for i, a := range idxs {
			if a < 0 {
				continue
			}
			for j := i + 1; j <= i+windowSize && j < len(idxs); j++ {
				b := idxs[j]
				if b < 0 {
					continue
				}
				coOccur[a][b]++
				coOccur[b][a]++
				totalCount[a]++
				totalCount[b]++
				grandTotal += 2
			}
		}
	}

	ppmi := computePPMI(coOccur, totalCount, grandTotal)
	projection := randomProjection(dim, n, seed)

	result := make(map[string][]float64, n)
	for word, idx := range vocab {
		result[word] = projectRow(ppmi[idx], projection, dim)
	}
	return result
}

// projectRow multiplies a sparse PPMI row by the random projection
// matrix. Co-occurrence indices are visited in sorted order rather than
// map iteration order so the floating-point summation is reproducible:
// float addition isn't associative, so summing the same terms in a
// different order can change the last few bits of the result, which
// would otherwise make BuildWordVectors' output depend on Go's
// randomized map iteration instead of only on its seed argument.
func projectRow(row map[int]float64, projection [][]float64, dim int) []float64 {
	indices := make([]int, 0, len(row))
	for b := range row {
		indices = append(indices, b)
	}
	sort.Ints(indices)

	vec := make([]float64, dim)
	for d := 0; d < dim; d++ {
		var sum float64
		for _, b := range indices {
			sum += projection[d][b] * row[b]
		}
		vec[d] = sum
	}
	return vec
}

// buildVocab keeps the maxVocab most frequent words, breaking ties
// alphabetically so the result (and therefore BuildWordVectors' output)
// is deterministic across runs on the same input.
func buildVocab(docFreq map[string]int, maxVocab int) map[string]int {
	words := make([]string, 0, len(docFreq))
	for w := range docFreq {
		words = append(words, w)
	}
	sort.Slice(words, func(i, j int) bool {
		if docFreq[words[i]] != docFreq[words[j]] {
			return docFreq[words[i]] > docFreq[words[j]]
		}
		return words[i] < words[j]
	})
	if maxVocab > 0 && len(words) > maxVocab {
		words = words[:maxVocab]
	}

	vocab := make(map[string]int, len(words))
	for i, w := range words {
		vocab[w] = i
	}
	return vocab
}

// computePPMI turns raw co-occurrence counts into positive pointwise
// mutual information: PMI(a,b) = log( P(a,b) / (P(a)*P(b)) ), clipped at
// zero since negative PMI (co-occurring less than chance) is noisy on
// small corpora and not useful signal for this embedding.
func computePPMI(coOccur []map[int]float64, totalCount []float64, grandTotal float64) []map[int]float64 {
	n := len(coOccur)
	ppmi := make([]map[int]float64, n)
	for a := 0; a < n; a++ {
		ppmi[a] = map[int]float64{}
		if totalCount[a] == 0 || grandTotal == 0 {
			continue
		}
		for b, count := range coOccur[a] {
			if count == 0 || totalCount[b] == 0 {
				continue
			}
			pab := count / grandTotal
			pa := totalCount[a] / grandTotal
			pb := totalCount[b] / grandTotal
			pmi := math.Log(pab / (pa * pb))
			if pmi > 0 {
				ppmi[a][b] = pmi
			}
		}
	}
	return ppmi
}

// randomProjection returns a dim x n matrix of iid standard-normal
// entries from a seeded RNG, so BuildWordVectors is deterministic for a
// given seed.
func randomProjection(dim, n int, seed int64) [][]float64 {
	rng := rand.New(rand.NewSource(seed))
	projection := make([][]float64, dim)
	for i := range projection {
		projection[i] = make([]float64, n)
		for j := range projection[i] {
			projection[i][j] = rng.NormFloat64()
		}
	}
	return projection
}

// AverageVector mean-pools the embedding vectors of tokens found in
// vectors, skipping unknown words. Returns a dim-length zero vector if
// none of the tokens are known (e.g. an empty document, or one entirely
// outside the trained vocabulary), so it always composes cleanly with
// TF-IDF features in a fixed-length feature vector.
func AverageVector(tokens []string, vectors map[string][]float64, dim int) []float64 {
	sum := make([]float64, dim)
	count := 0
	for _, tok := range tokens {
		v, ok := vectors[tok]
		if !ok {
			continue
		}
		for i, x := range v {
			sum[i] += x
		}
		count++
	}
	if count == 0 {
		return sum
	}
	for i := range sum {
		sum[i] /= float64(count)
	}
	return sum
}

// Embedder bundles a trained word-vector table with its dimension so
// callers can turn raw document text directly into a semantic feature
// vector, the same way tfidf.Vectorizer turns text into a TF-IDF vector.
type Embedder struct {
	Vectors map[string][]float64 `json:"vectors"`
	Dim     int                  `json:"dim"`
}

// Transform tokenizes text and mean-pools its known word vectors. A
// zero-value Embedder (Dim == 0, e.g. one loaded from a model saved
// before this feature existed) returns an empty slice, so appending its
// result to a TF-IDF vector is a no-op and older models keep working
// unchanged.
func (e *Embedder) Transform(doc string) []float64 {
	if e.Dim == 0 {
		return []float64{}
	}
	return AverageVector(text.Tokenize(doc), e.Vectors, e.Dim)
}
