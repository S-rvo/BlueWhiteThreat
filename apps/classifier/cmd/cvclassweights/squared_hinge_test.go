package main

import (
	"math"
	"testing"
)

func TestSquaredHingeSeparatesWithUnregularizedBias(t *testing.T) {
	x := [][]float64{{0}, {0}, {1}, {1}}
	lr, err := trainSquaredHinge(x, []float64{0, 0, 1, 1}, []float64{1, 1, 1, 1}, 1200, .005)
	if err != nil {
		t.Fatal(err)
	}
	if lr.Predict([]float64{0}) >= .5 || lr.Predict([]float64{1}) <= .5 {
		t.Fatalf("did not separate: %+v", lr)
	}
	// Duplicating rows and preserving their relative weights must leave the
	// mean-loss gradient unchanged; class prevalence is not a hidden step size.
	lr2, err := trainSquaredHinge(append(x, x...), []float64{0, 0, 1, 1, 0, 0, 1, 1}, []float64{1, 1, 1, 1, 1, 1, 1, 1}, 1200, .005)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(lr.Weights[0]-lr2.Weights[0]) > 1e-12 || math.Abs(lr.Bias-lr2.Bias) > 1e-12 {
		t.Fatal("mean loss depends on replicated sample count")
	}
}

func TestSquaredHingeZeroWeightDoesNotAffectFit(t *testing.T) {
	a, err := trainSquaredHinge([][]float64{{-1}, {1}}, []float64{0, 1}, []float64{1, 1}, 200, .005)
	if err != nil {
		t.Fatal(err)
	}
	b, err := trainSquaredHinge([][]float64{{-1}, {1}, {1}}, []float64{0, 1, 0}, []float64{1, 1, 0}, 200, .005)
	if err != nil {
		t.Fatal(err)
	}
	if a.Weights[0] != b.Weights[0] || a.Bias != b.Bias {
		t.Fatal("zero weight changed training")
	}
	if _, err := trainSquaredHinge([][]float64{{1}}, []float64{1}, []float64{0}, 200, .005); err == nil {
		t.Fatal("zero total weight accepted")
	}
	if _, err := trainSquaredHinge([][]float64{{math.NaN()}}, []float64{1}, []float64{1}, 200, .005); err == nil {
		t.Fatal("nonfinite feature accepted")
	}
}
