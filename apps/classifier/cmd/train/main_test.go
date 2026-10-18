// apps/classifier/cmd/train/main_test.go
package main

import "testing"

// TestPickThreshold_RejectsPrecisionFloorWithDegenerateRecall builds scores
// where the only threshold reaching targetPrecision does so by predicting a
// single positive out of ten (recall 0.1) — a degenerate operating point a
// small, noisy validation set can produce. A much better-rounded threshold
// exists (precision 0.833, recall 1.0, f1 0.909) but never reaches the
// precision floor. pickThreshold must reject the degenerate floor-meeting
// threshold once it falls under minRecall, and fall back to the
// F1-maximizing threshold instead of returning a near-zero-recall cutoff.
func TestPickThreshold_RejectsPrecisionFloorWithDegenerateRecall(t *testing.T) {
	var scores, labels []float64

	// One positive scored high enough to be the only example separable at
	// an extreme threshold.
	scores = append(scores, 0.99)
	labels = append(labels, 1)

	// Nine more positives, all scored just above the well-supported cutoff.
	for i := 0; i < 9; i++ {
		scores = append(scores, 0.605)
		labels = append(labels, 1)
	}

	// Two negatives contaminate the well-supported cutoff (dropping its
	// precision below the target) but not the extreme one.
	for i := 0; i < 2; i++ {
		scores = append(scores, 0.655)
		labels = append(labels, 0)
	}

	// Eight negatives score low and never contaminate any candidate
	// threshold under test.
	for i := 0; i < 8; i++ {
		scores = append(scores, 0.1)
		labels = append(labels, 0)
	}

	got := pickThreshold(scores, labels)
	// The exact-score search picks the highest boundary on this plateau.
	if got != 0.605 {
		t.Fatalf("expected boundary 0.605, got %.9f", got)
	}

	_, precision, recall, _ := confusionAt(scores, labels, got)
	if precision < 0.8 || recall < 0.9 {
		t.Fatalf("expected the well-rounded operating point (precision~0.833, recall~1.0), got precision=%.3f recall=%.3f", precision, recall)
	}
}

func TestPickThreshold_CompressedScores(t *testing.T) {
	scores := []float64{0.493, 0.495, 0.497, 0.499}
	labels := []float64{0, 0, 1, 1}
	got := pickThreshold(scores, labels)
	if got != 0.497 {
		t.Fatalf("missed attainable perfect separation: %.9f", got)
	}
}

func TestCandidateOutputPath(t *testing.T) {
	t.Setenv("MODEL_PATH", "live.json")
	t.Setenv("CANDIDATE_MODEL_PATH", "")
	got, err := candidateOutputPath("provisional-example")
	if err != nil || got != "model_provisional-example.json" {
		t.Fatalf("default: %q %v", got, err)
	}
	for _, path := range []string{"live.json", "./live.json", "model.json", "nested/model.json"} {
		t.Setenv("CANDIDATE_MODEL_PATH", path)
		if _, err := candidateOutputPath("provisional-example"); err == nil {
			t.Fatalf("accepted live path %q", path)
		}
	}
	t.Setenv("CANDIDATE_MODEL_PATH", "candidate.json")
	if got, err := candidateOutputPath("provisional-example"); err != nil || got != "candidate.json" {
		t.Fatalf("candidate: %q %v", got, err)
	}
}

func TestValidationSummaryDistinguishesFallback(t *testing.T) {
	for _, tc := range []struct {
		name           string
		scores, labels []float64
		met            bool
		rule           string
	}{
		{"constrained", []float64{.9, .8, .1}, []float64{1, 1, 0}, true, "maximum_recall_at_precision_and_recall_constraints"},
		{"fallback", []float64{.5, .5}, []float64{1, 0}, false, "maximum_f1_fallback"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			threshold := pickThreshold(tc.scores, tc.labels)
			got := validationSummary(tc.scores, tc.labels, threshold)
			if got.ObjectiveMet != tc.met || got.SelectionRule != tc.rule || got.Documents != len(tc.labels) || got.LabelAuthority != "input_csv_authority_unspecified" {
				t.Fatalf("summary: %+v", got)
			}
		})
	}
}
