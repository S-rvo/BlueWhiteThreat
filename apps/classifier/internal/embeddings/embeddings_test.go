// apps/classifier/internal/embeddings/embeddings_test.go
package embeddings

import (
	"math"
	"testing"
)

func cosine(a, b []float64) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

func repeat(docs []string, times int) []string {
	out := make([]string, 0, len(docs)*times)
	for i := 0; i < times; i++ {
		out = append(out, docs...)
	}
	return out
}

func TestBuildWordVectors_CoOccurringWordsAreCloser(t *testing.T) {
	docs := repeat([]string{
		"malware infects systems quickly",
		"ransomware infects systems quickly",
		"malware encrypts files rapidly",
		"ransomware encrypts files rapidly",
		"malware spreads across networks",
		"ransomware spreads across networks",
		"quarterly earnings grew steadily",
		"quarterly profit grew steadily",
		"annual earnings rose sharply",
		"annual profit rose sharply",
	}, 20)

	vectors := BuildWordVectors(docs, 4, 16, 50, 42)

	malware, ok := vectors["malware"]
	if !ok {
		t.Fatalf("expected vector for %q", "malware")
	}
	ransomware, ok := vectors["ransomware"]
	if !ok {
		t.Fatalf("expected vector for %q", "ransomware")
	}
	quarterly, ok := vectors["quarterly"]
	if !ok {
		t.Fatalf("expected vector for %q", "quarterly")
	}

	simRelated := cosine(malware, ransomware)
	simUnrelated := cosine(malware, quarterly)

	if simRelated <= simUnrelated {
		t.Errorf("expected cosine(malware, ransomware)=%.4f > cosine(malware, quarterly)=%.4f", simRelated, simUnrelated)
	}
}

func TestBuildWordVectors_DeterministicGivenSeed(t *testing.T) {
	docs := repeat([]string{
		"malware infects systems quickly",
		"ransomware encrypts files rapidly",
	}, 5)

	a := BuildWordVectors(docs, 4, 8, 50, 7)
	b := BuildWordVectors(docs, 4, 8, 50, 7)

	for word, va := range a {
		vb, ok := b[word]
		if !ok {
			t.Fatalf("word %q missing from second run", word)
		}
		for i := range va {
			if va[i] != vb[i] {
				t.Errorf("vector for %q differs between runs with the same seed: %v vs %v", word, va, vb)
			}
		}
	}
}

func TestBuildWordVectors_EmptyInputReturnsEmptyMap(t *testing.T) {
	vectors := BuildWordVectors(nil, 4, 8, 50, 42)
	if len(vectors) != 0 {
		t.Errorf("expected empty map for empty input, got %d entries", len(vectors))
	}
}

func TestAverageVector_UnknownTokensSkipped(t *testing.T) {
	vectors := map[string][]float64{
		"malware": {1, 0, 0},
		"spreads": {0, 1, 0},
	}

	got := AverageVector([]string{"malware", "spreads", "zzzznotindict"}, vectors, 3)
	want := []float64{0.5, 0.5, 0}

	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-9 {
			t.Errorf("AverageVector() = %v, want %v", got, want)
		}
	}
}

func TestAverageVector_NoKnownTokensReturnsZeroVector(t *testing.T) {
	vectors := map[string][]float64{
		"malware": {1, 2, 3},
	}

	got := AverageVector([]string{"unknown"}, vectors, 3)
	for i, v := range got {
		if v != 0 {
			t.Errorf("AverageVector()[%d] = %v, want 0", i, v)
		}
	}
}

func TestEmbedder_TransformAveragesKnownWordsInText(t *testing.T) {
	e := &Embedder{
		Vectors: map[string][]float64{
			"malware": {1, 0},
			"spreads": {0, 1},
		},
		Dim: 2,
	}

	got := e.Transform("malware spreads quickly")
	want := []float64{0.5, 0.5}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-9 {
			t.Errorf("Transform() = %v, want %v", got, want)
		}
	}
}

func TestEmbedder_TransformWithZeroDimReturnsEmptySlice(t *testing.T) {
	e := &Embedder{Vectors: map[string][]float64{}, Dim: 0}
	got := e.Transform("anything at all")
	if len(got) != 0 {
		t.Errorf("expected empty slice for zero-dim embedder, got %v", got)
	}
}
