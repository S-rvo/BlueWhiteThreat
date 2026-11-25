package main

import (
	"math"
	"testing"
)

func TestCappedHingeHasBoundedInfluenceForWrongMargin(t *testing.T) {
	// Update 1: w=-.425,b=.05. Positive row then has signed margin
	// -.375, so its violation 1.375 is capped at 1. Negative row has
	// signed margin 8.45 and contributes nothing to the second update.
	x := [][]float64{{1}, {20}}
	lr, err := trainMarginLoss(x, []float64{1, 0}, []float64{3, 1}, 2, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(lr.Weights[0]-(-.35)) > 1e-12 || math.Abs(lr.Bias-.125) > 1e-12 {
		t.Fatalf("uncapped or incorrect update: %+v", lr)
	}
}

func TestCappedHingeSeparatesAndRejectsNonfiniteFeatures(t *testing.T) {
	lr, err := trainMarginLoss([][]float64{{-1}, {1}}, []float64{0, 1}, []float64{1, 1}, 1200, .005, true)
	if err != nil {
		t.Fatal(err)
	}
	if lr.Predict([]float64{-1}) >= .5 || lr.Predict([]float64{1}) <= .5 {
		t.Fatal("did not separate")
	}
	if _, err := trainMarginLoss([][]float64{{math.NaN()}}, []float64{1}, []float64{1}, 10, .005, true); err == nil {
		t.Fatal("accepted NaN")
	}
}
