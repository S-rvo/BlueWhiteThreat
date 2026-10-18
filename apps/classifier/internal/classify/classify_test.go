// apps/classifier/internal/classify/classify_test.go
package classify

import (
	"math"
	"testing"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/model"
)

func TestNoFeaturesNeedsReviewEvenWhenBiasIsFarFromThreshold(t *testing.T) {
	for _, bias := range []float64{-10, 10} {
		m := &model.Model{Vocabulary: map[string]int{"known": 0}, IDF: []float64{1}, Weights: []float64{1}, Bias: bias, Threshold: .5, Version: "no-features-test"}
		classifier := New(m)
		for _, text := range []string{"", "! a", "unseen vocabulary"} {
			result := classifier.Classify(text)
			if !result.NeedsReview {
				t.Fatalf("no-feature text escaped review with bias %g", bias)
			}
			expected := 1 / (1 + math.Exp(-bias))
			if result.Score != expected || result.Relevant != (expected >= .5) {
				t.Fatal("review guard changed score or verdict")
			}
			if result.ModelVersion != m.Version {
				t.Fatal("model version changed")
			}
		}
		if classifier.Classify("known").NeedsReview {
			t.Fatal("guard caught a nonzero feature vector")
		}
	}
}

func TestClassify(t *testing.T) {
	// Hand-built tiny model: vocabulary of one term, positive weight,
	// so any document containing "ransomware" scores above threshold
	// and any document without it scores at the bias (below threshold).
	m := &model.Model{
		Vocabulary: map[string]int{"ransomware": 0},
		IDF:        []float64{1.0},
		Weights:    []float64{5.0},
		Bias:       -1.0,
		Threshold:  0.5,
		Version:    "unit-test",
	}

	c := New(m)

	relevant := c.Classify("a new ransomware campaign is spreading")
	if !relevant.Relevant {
		t.Fatalf("expected relevant=true, got score=%f", relevant.Score)
	}
	if relevant.ModelVersion != "unit-test" {
		t.Fatalf("expected model version to be propagated, got %q", relevant.ModelVersion)
	}

	notRelevant := c.Classify("quarterly earnings exceeded expectations")
	if notRelevant.Relevant {
		t.Fatalf("expected relevant=false, got score=%f", notRelevant.Score)
	}
}

func TestClassify_AllZeroVector(t *testing.T) {
	// A document with no tokens in the vocabulary produces an all-zero
	// TF-IDF vector, so the score should fall back to sigmoid(Bias).
	m := &model.Model{
		Vocabulary: map[string]int{"ransomware": 0},
		IDF:        []float64{1.0},
		Weights:    []float64{5.0},
		Bias:       -1.0,
		Threshold:  0.5,
		Version:    "unit-test",
	}

	c := New(m)

	result := c.Classify("completely unrelated text with no vocabulary hits")
	if result.Relevant {
		t.Fatalf("expected relevant=false for all-zero vector, got score=%f", result.Score)
	}
}

func TestClassify_NeedsReviewNearThreshold(t *testing.T) {
	// Threshold 0.5, ReviewMargin 0.1: scores within [0.4, 0.6] are
	// borderline calls the model isn't confident about, so they should be
	// flagged for human review rather than silently trusted either way.
	m := &model.Model{
		Vocabulary: map[string]int{},
		IDF:        []float64{},
		Weights:    []float64{},
		Bias:       0, // sigmoid(0) == 0.5 == threshold exactly
		Threshold:  0.5,
		Version:    "unit-test",
	}

	c := New(m)

	result := c.Classify("any text at all")
	if !result.NeedsReview {
		t.Fatalf("expected needs_review=true for score=%f within margin of threshold=0.5", result.Score)
	}
}

func TestClassify_ConfidentScoreNotFlaggedForReview(t *testing.T) {
	// A strongly positive weight pushes the score far from the threshold,
	// so a confident classification should not be flagged for review.
	m := &model.Model{
		Vocabulary: map[string]int{"ransomware": 0},
		IDF:        []float64{1.0},
		Weights:    []float64{5.0},
		Bias:       -1.0,
		Threshold:  0.5,
		Version:    "unit-test",
	}

	c := New(m)

	result := c.Classify("a new ransomware campaign is spreading")
	if result.NeedsReview {
		t.Fatalf("expected needs_review=false for confident score=%f", result.Score)
	}
}

func TestClassify_ScoreEqualsThreshold(t *testing.T) {
	// An empty vocabulary always yields a zero vector, so the score is
	// exactly sigmoid(Bias). With Bias=0, sigmoid(0)==0.5 exactly, landing
	// precisely on Threshold=0.5. Classify documents Relevant as true when
	// score >= threshold, so this exact-boundary case must be relevant.
	m := &model.Model{
		Vocabulary: map[string]int{},
		IDF:        []float64{},
		Weights:    []float64{},
		Bias:       0,
		Threshold:  0.5,
		Version:    "unit-test",
	}

	c := New(m)

	result := c.Classify("any text at all")
	if !result.Relevant {
		t.Fatalf("expected relevant=true when score==threshold, got score=%f", result.Score)
	}
}

