package main

import (
	"math"
	"reflect"
	"testing"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/tfidf"
)

func TestBinaryTermsIgnoreFrequencyAndPreserveIDF(t *testing.T) {
	v := &tfidf.Vectorizer{Vocabulary: map[string]int{"alpha": 0, "beta": 1}, IDF: []float64{2, 1}}
	a := transformTerms(v, "alpha beta", "binary")
	b := transformTerms(v, "alpha alpha alpha beta", "binary")
	if !reflect.DeepEqual(a, b) || math.Abs(a[0]-2/math.Sqrt(5)) > 1e-12 || math.Abs(a[1]-1/math.Sqrt(5)) > 1e-12 {
		t.Fatalf("binary mode changed presence/IDF: %v %v", a, b)
	}
	if !reflect.DeepEqual(transformTerms(v, "unknown", "binary"), []float64{0, 0}) {
		t.Fatal("unknown terms should remain zero")
	}
	if !reflect.DeepEqual(transformTerms(v, "alpha alpha beta", "log"), v.Transform("alpha alpha beta")) {
		t.Fatal("default transform changed")
	}
}

func TestSaturatedTermsUseExperimentalTransform(t *testing.T) {
	v := &tfidf.Vectorizer{Vocabulary: map[string]int{"alpha": 0, "beta": 1}, IDF: []float64{2, 1}}
	if got := transformTerms(v, "alpha alpha alpha beta", "saturated"); !reflect.DeepEqual(got, v.TransformSaturated("alpha alpha alpha beta")) {
		t.Fatalf("saturated mode was not routed: %v", got)
	}
}

func TestBlockPoolingPreservesTailAndDefault(t *testing.T) {
	v := &tfidf.Vectorizer{Vocabulary: map[string]int{"alpha": 0, "beta": 1}, IDF: []float64{2, 1}}
	doc := "alpha alpha beta"
	if !reflect.DeepEqual(transformBlocks(v, doc, "log", 0), v.Transform(doc)) {
		t.Fatal("default changed")
	}
	x := transformBlocks(v, doc, "log", 2)
	if math.Abs(x[0]-1/math.Sqrt(2)) > 1e-12 || math.Abs(x[1]-1/math.Sqrt(2)) > 1e-12 {
		t.Fatalf("tail block lost or pooled vector not normalized: %v", x)
	}
	if !reflect.DeepEqual(transformBlocks(v, "", "log", 2), []float64{0, 0}) {
		t.Fatal("empty document should remain zero")
	}
	if !reflect.DeepEqual(transformBlocks(v, "alpha beta", "log", 3), v.Transform("alpha beta")) {
		t.Fatal("short document changed")
	}
}

func TestTrainingSubsetIsStratifiedNestedAndExcludesHeldout(t *testing.T) {
	groups := []group{{id: "a", label: 0}, {id: "b", label: 0}, {id: "c", label: 1}, {id: "d", label: 1}, {id: "heldout", label: 1}}
	assigned := []int{0, 0, 0, 0, 1}
	full := trainingSubset(groups, assigned, 1, 1, 42)
	if !reflect.DeepEqual(full, []int{0, 1, 2, 3}) {
		t.Fatalf("full train changed: %v", full)
	}
	half := trainingSubset(groups, assigned, 1, 0.5, 42)
	if len(half) != 2 || groups[half[0]].label != 0 || groups[half[1]].label != 1 {
		t.Fatalf("subset lost class balance: %v", half)
	}
	for _, index := range half {
		if assigned[index] == 1 {
			t.Fatal("heldout leaked into train")
		}
	}
	if !reflect.DeepEqual(half, trainingSubset(groups, assigned, 1, 0.5, 42)) {
		t.Fatal("subset is nondeterministic")
	}
}

func TestDeduplicateLongLinesPreservesShortCellsAndFirstOccurrence(t *testing.T) {
	doc := "Heading\nA long repeated navigation sentence here\ncell\n A  LONG repeated navigation sentence here \ncell\nA different sentence with useful detail"
	want := "Heading\nA long repeated navigation sentence here\ncell\ncell\nA different sentence with useful detail"
	if got := deduplicateLongLines(doc); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got := deduplicateLongLines(want); got != want {
		t.Fatal("cleaner must be idempotent")
	}
}

func TestAveragePrecisionTreatsTiedScoresAsOneGroup(t *testing.T) {
	first := metrics([]float64{0.9, 0.9, 0.2}, []float64{1, 0, 1})
	permuted := metrics([]float64{0.9, 0.9, 0.2}, []float64{0, 1, 1})
	want := 7.0 / 12.0
	if math.Abs(first.AveragePrecision-want) > 1e-12 {
		t.Fatalf("average precision for tied positive/negative scores = %v, want %v", first.AveragePrecision, want)
	}
	if first.AveragePrecision != permuted.AveragePrecision {
		t.Fatalf("average precision changed with row order: %v vs %v", first.AveragePrecision, permuted.AveragePrecision)
	}
}
