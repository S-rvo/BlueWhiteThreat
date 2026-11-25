package classify

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/model"
)

// Historical artifacts encode bounded 3–5 character fragments and raw TF.
// This unequal repetition also detects an accidental switch to log1p TF.
func TestLegacyArtifactFeatureEncoding(t *testing.T) {
	characters := true
	m := &model.Model{CharNgrams: &characters, Vocabulary: map[string]int{"@c:^ab": 0, "@c:^xy": 1}, IDF: []float64{1, 1}, Weights: []float64{2, -1}, Threshold: .5}
	got := New(m).Classify("abc abc xyz").Score
	want := 1 / (1 + math.Exp(-3/math.Sqrt(5)))
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("score %g, want %g", got, want)
	}
	m.SublinearTF = true
	a := 1 + math.Log(2)
	want = 1 / (1 + math.Exp(-(2*a-1)/math.Sqrt(a*a+1)))
	if got = New(m).Classify("abc abc xyz").Score; math.Abs(got-want) > 1e-12 {
		t.Fatalf("sublinear score %g, want %g", got, want)
	}
	characters = false
	if got = New(m).Classify("abc abc xyz").Score; got != .5 {
		t.Fatalf("disabled character features: %g", got)
	}
}

// Loading and saving an artifact must not silently change the vectorizer family.
func TestArtifactRoundTripPreservesHistoricalInference(t *testing.T) {
	for _, value := range []string{`"char_ngrams":true,`, `"char_ngrams":false,`, ``} {
		t.Run(value, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "artifact.json")
			raw := `{` + value + `"sublinear_tf":true,"vocabulary":{"@c:^ab":0,"@c:^xy":1},"idf":[1,1],"weights":[2,-1],"bias":0,"threshold":0.5}`
			if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			before, err := model.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			expected := New(before).Classify("abc abc xyz").Score
			if err = model.Save(path, before); err != nil {
				t.Fatal(err)
			}
			after, err := model.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if (before.CharNgrams == nil) != (after.CharNgrams == nil) {
				t.Fatal("vectorizer family changed")
			}
			if got := New(after).Classify("abc abc xyz").Score; got != expected {
				t.Fatalf("score changed after persistence: %g != %g", got, expected)
			}
		})
	}
}
