// apps/classifier/internal/tfidf/vectorizer_test.go
package tfidf

import (
	"fmt"
	"math"
	"testing"
)

func TestFitTransformRoundTrip(t *testing.T) {
	docs := []string{
		"malware attack network",
		"malware ransomware attack",
		"network defense security",
	}

	v := Fit(docs)

	for _, tok := range []string{"malware", "attack", "network", "ransomware", "defense", "security"} {
		if _, ok := v.Vocabulary[tok]; !ok {
			t.Fatalf("expected vocabulary to contain %q, vocab=%v", tok, v.Vocabulary)
		}
	}

	// "malware" appears in docs 0 and 1 (df=2), n=3.
	// idf = log((3+1)/(2+1)) + 1
	wantIDF := math.Log(4.0/3.0) + 1
	idx := v.Vocabulary["malware"]
	if math.Abs(v.IDF[idx]-wantIDF) > 1e-9 {
		t.Fatalf("idf for malware = %v, want %v", v.IDF[idx], wantIDF)
	}

	// Transform doc0: "malware attack network" -> tf=1 for each of its tokens.
	vec := v.Transform(docs[0])
	wantVal := 1.0 * wantIDF
	if math.Abs(vec[idx]-wantVal) > 1e-9 {
		t.Fatalf("transform value for malware in doc0 = %v, want %v", vec[idx], wantVal)
	}
	if len(vec) != len(v.Vocabulary) {
		t.Fatalf("vector length = %d, want %d", len(vec), len(v.Vocabulary))
	}
}

func TestFitDeterminism(t *testing.T) {
	docs := []string{
		"alpha beta gamma",
		"beta gamma delta",
		"gamma delta epsilon",
		"alpha delta epsilon",
	}

	v1 := Fit(docs)
	v2 := Fit(docs)

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
	var docs []string
	for i := 0; i < 100; i++ {
		docs = append(docs, fmt.Sprintf("tok%d tok%d tok%d common", i%10, (i+1)%10, (i+2)%10))
	}
	for i := 0; i < MaxVocabularySize+500; i++ {
		docs = append(docs, fmt.Sprintf("uniq%d", i))
	}

	v := Fit(docs)

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
	v := Fit(docs)

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

func TestTransformOnManuallyConstructedVectorizer(t *testing.T) {
	v := &Vectorizer{
		Vocabulary: map[string]int{
			"malware": 0,
			"attack":  1,
		},
		IDF: []float64{2.0, 0.5},
	}

	vec := v.Transform("malware malware attack unknown")
	want := []float64{4.0, 0.5}
	if len(vec) != len(want) {
		t.Fatalf("len(vec) = %d, want %d", len(vec), len(want))
	}
	for i := range want {
		if math.Abs(vec[i]-want[i]) > 1e-9 {
			t.Fatalf("vec[%d] = %v, want %v", i, vec[i], want[i])
		}
	}
}
