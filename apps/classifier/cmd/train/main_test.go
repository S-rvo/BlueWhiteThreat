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
	// F1 is flat at 0.909 across the whole well-supported plateau
	// (0.11-0.60); pickThreshold returns the first candidate reaching that
	// max, so assert on the plateau's bounds rather than one exact value.
	if got < 0.10 || got > 0.60 {
		t.Fatalf("expected a threshold on the well-supported F1-max plateau (0.10-0.60), got %.2f", got)
	}
	_, precision, recall, _ := confusionAt(scores, labels, got)
	if precision < 0.8 || recall < 0.9 {
		t.Fatalf("expected the well-rounded operating point (precision~0.833, recall~1.0), got precision=%.3f recall=%.3f", precision, recall)
	}
}
