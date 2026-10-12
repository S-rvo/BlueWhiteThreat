// apps/classifier/internal/classify/classify_test.go
package classify

import (
	"testing"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/model"
)

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
