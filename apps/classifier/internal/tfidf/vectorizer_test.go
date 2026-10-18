package tfidf

import (
	"fmt"
	"math"
	"testing"
)

func zeros(n int) []float64 {
	return make([]float64, n)
}

func TestFitIncludesCharacterNgrams(t *testing.T) {
	// Dark-web content is rife with l33tspeak/typo variants of the same
	// word ("ransomw4re", "r4nsomware"). Character n-grams give the model
	// a feature that still overlaps heavily between such variants, where a
	// pure word-token vocabulary would treat them as entirely unrelated.
	docs := []string{
		"a ransomware gang leaked data",
		"a ransomware gang leaked data",
		"unrelated quarterly earnings report",
	}
	v := Fit(docs, zeros(len(docs)))

	if _, ok := v.Vocabulary["c_rans"]; !ok {
		t.Fatalf("expected vocabulary to contain character 4-gram %q, vocab=%v", "c_rans", v.Vocabulary)
	}

	vec := v.Transform("ransomware")
	idx := v.Vocabulary["c_rans"]
	if vec[idx] <= 0 {
		t.Fatalf("expected Transform(%q) to score character n-gram %q > 0, got %v", "ransomware", "c_rans", vec[idx])
	}
}

func TestFitTransformRoundTrip(t *testing.T) {
	docs := []string{
		"malware attack network",
		"malware ransomware attack",
		"network defense security",
	}

	v := Fit(docs, zeros(len(docs)))

	for _, tok := range []string{"malware", "attack", "network", "ransomware", "defense", "security"} {
		if _, ok := v.Vocabulary[tok]; !ok {
			t.Fatalf("expected vocabulary to contain %q, vocab=%v", tok, v.Vocabulary)
		}
	}
	// Adjacent-pair bigrams are also indexed.
	if _, ok := v.Vocabulary["malware_attack"]; !ok {
		t.Fatalf("expected vocabulary to contain bigram %q", "malware_attack")
	}

	// "malware" appears in docs 0 and 1 (df=2), n=3.
	wantIDF := math.Log(4.0/3.0) + 1
	idx := v.Vocabulary["malware"]
	if math.Abs(v.IDF[idx]-wantIDF) > 1e-9 {
		t.Fatalf("idf for malware = %v, want %v", v.IDF[idx], wantIDF)
	}

	vec := v.Transform(docs[0])
	if len(vec) != len(v.Vocabulary) {
		t.Fatalf("vector length = %d, want %d", len(vec), len(v.Vocabulary))
	}
	if vec[idx] <= 0 {
		t.Fatalf("transform value for malware in doc0 = %v, want > 0", vec[idx])
	}

	// Transform output is L2-normalized: the vector's norm is 1 for any
	// document that has at least one in-vocabulary token.
	var sumSq float64
	for _, val := range vec {
		sumSq += val * val
	}
	if math.Abs(math.Sqrt(sumSq)-1.0) > 1e-9 {
		t.Fatalf("||Transform(doc0)|| = %v, want 1.0", math.Sqrt(sumSq))
	}
}

func TestFitDeterminism(t *testing.T) {
	docs := []string{
		"alpha beta gamma",
		"beta gamma delta",
		"gamma delta epsilon",
		"alpha delta epsilon",
	}
	labels := zeros(len(docs))

	v1 := Fit(docs, labels)
	v2 := Fit(docs, labels)

	if len(v1.Vocabulary) != len(v2.Vocabulary) {
		t.Fatalf("vocab size mismatch: %d vs %d", len(v1.Vocabulary), len(v2.Vocabulary))
	}
	for tok, idx := range v1.Vocabulary {
		idx2, ok := v2.Vocabulary[tok]
		if !ok || idx2 != idx {
			t.Fatalf("vocabulary mismatch for token %q: %d vs %v (ok=%v)", tok, idx, idx2, ok)
		}
	}
	for i := range v1.IDF {
		if math.Abs(v1.IDF[i]-v2.IDF[i]) > 1e-12 {
			t.Fatalf("idf mismatch at index %d: %v vs %v", i, v1.IDF[i], v2.IDF[i])
		}
	}
}

func TestVocabularyCapping(t *testing.T) {
	// Build a corpus with more than MaxVocabularySize distinct tokens.
	// Tokens tok0..tok99 each appear in 3 documents (high df);
	// tokens uniqN each appear in exactly 1 document (low df) and there are
	// enough of them to exceed MaxVocabularySize distinct tokens overall.
	// All documents share the same label, so chi-square association is 0
	// everywhere and ranking falls back to document frequency.
	var docs []string
	for i := 0; i < 100; i++ {
		docs = append(docs, fmt.Sprintf("tok%d tok%d tok%d common", i%10, (i+1)%10, (i+2)%10))
	}
	for i := 0; i < MaxVocabularySize+500; i++ {
		docs = append(docs, fmt.Sprintf("uniq%d", i))
	}

	v := Fit(docs, zeros(len(docs)))

	if len(v.Vocabulary) != MaxVocabularySize {
		t.Fatalf("vocabulary size = %d, want %d", len(v.Vocabulary), MaxVocabularySize)
	}

	// High-document-frequency tokens must be retained over the low-df unique
	// tokens.
	if _, ok := v.Vocabulary["common"]; !ok {
		t.Fatalf("expected high-df token %q to be retained", "common")
	}
	for i := 0; i < 10; i++ {
		tok := fmt.Sprintf("tok%d", i)
		if _, ok := v.Vocabulary[tok]; !ok {
			t.Fatalf("expected high-df token %q to be retained", tok)
		}
	}
}

func TestTransformEmptyAndOOV(t *testing.T) {
	docs := []string{"alpha beta", "beta gamma"}
	v := Fit(docs, zeros(len(docs)))

	cases := []string{"", "zzz unknown words only"}
	for _, doc := range cases {
		vec := v.Transform(doc)
		if len(vec) != len(v.Vocabulary) {
			t.Fatalf("Transform(%q) length = %d, want %d", doc, len(vec), len(v.Vocabulary))
		}
		for i, val := range vec {
			if val != 0.0 {
				t.Fatalf("Transform(%q)[%d] = %v, want 0.0", doc, i, val)
			}
		}
	}
}

func TestTransformIsL2Normalized(t *testing.T) {
	v := &Vectorizer{
		Vocabulary: map[string]int{
			"malware": 0,
			"attack":  1,
		},
		IDF: []float64{2.0, 0.5},
	}

	vec := v.Transform("malware malware attack unknown")
	// TF is log-scaled (log1p(count)) before multiplying by IDF, so a term
	// repeated many times doesn't dominate linearly - "malware" appearing
	// twice shouldn't count as literally twice the signal of once. Raw
	// (pre-normalization) values: malware = log1p(2)*2.0, attack = log1p(1)*0.5.
	rawMalware := math.Log1p(2) * 2.0
	rawAttack := math.Log1p(1) * 0.5
	rawNorm := math.Sqrt(rawMalware*rawMalware + rawAttack*rawAttack)
	want := []float64{rawMalware / rawNorm, rawAttack / rawNorm}

	if len(vec) != len(want) {
		t.Fatalf("len(vec) = %d, want %d", len(vec), len(want))
	}
	for i := range want {
		if math.Abs(vec[i]-want[i]) > 1e-9 {
			t.Fatalf("vec[%d] = %v, want %v", i, vec[i], want[i])
		}
	}

	var sumSq float64
	for _, val := range vec {
		sumSq += val * val
	}
	if math.Abs(math.Sqrt(sumSq)-1.0) > 1e-9 {
		t.Fatalf("||vec|| = %v, want 1.0", math.Sqrt(sumSq))
	}
}

