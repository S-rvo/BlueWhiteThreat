// apps/classifier/internal/model/calibration_test.go
package model

import "testing"

func TestIdentityCalibrator_LeavesScoreUnchanged(t *testing.T) {
	c := Identity()
	for _, p := range []float64{0.01, 0.2, 0.5, 0.8, 0.99} {
		got := c.Apply(p)
		if diff := got - p; diff > 1e-6 || diff < -1e-6 {
			t.Fatalf("Identity().Apply(%v) = %v, want %v", p, got, p)
		}
	}
}

func TestZeroValueCalibrator_BehavesAsIdentity(t *testing.T) {
	var c Calibrator // zero value, e.g. an older model.json with no calibration field
	got := c.Apply(0.73)
	if diff := got - 0.73; diff > 1e-6 || diff < -1e-6 {
		t.Fatalf("zero-value Calibrator.Apply(0.73) = %v, want 0.73 (identity)", got)
	}
}

func TestFitPlattScaling_CorrectsOverconfidentScores(t *testing.T) {
	// Simulate a model trained with balanced class weights on an imbalanced
	// dataset: raw scores are systematically higher than the true positive
	// rate at that score (classic symptom of balanced-loss training).
	// Overconfident raw scores for genuine positives, modest raw scores for
	// genuine negatives, but every true label is 0 except a couple of very
	// high-raw-score cases — calibration should pull high raw scores for
	// negatives down toward the actual observed positive rate.
	rawScores := []float64{0.95, 0.9, 0.92, 0.88, 0.85, 0.3, 0.2, 0.25, 0.1, 0.15}
	labels := []float64{1, 0, 1, 0, 0, 0, 0, 0, 0, 0}

	c := FitPlattScaling(rawScores, labels, 2000, 0.1)

	// The calibrated score for a raw 0.9 (where only 2/5 high-score examples
	// are truly positive) should be pulled down from the raw value.
	calibrated := c.Apply(0.9)
	if calibrated >= 0.9 {
		t.Fatalf("expected calibration to reduce overconfident score 0.9, got %v", calibrated)
	}

	// Calibration must preserve the ranking of raw scores (monotonic), so
	// it never flips which of two documents looks more relevant.
	a := c.Apply(0.95)
	b := c.Apply(0.3)
	if a <= b {
		t.Fatalf("expected calibration to preserve ordering (0.95 > 0.3 raw), got calibrated %v <= %v", a, b)
	}
}

func TestFitPlattScaling_EmptyInputReturnsIdentity(t *testing.T) {
	c := FitPlattScaling(nil, nil, 100, 0.1)
	got := c.Apply(0.42)
	if diff := got - 0.42; diff > 1e-6 || diff < -1e-6 {
		t.Fatalf("FitPlattScaling(nil, nil, ...).Apply(0.42) = %v, want 0.42 (identity)", got)
	}
}
