package main

import "testing"

func TestPrecisionObjectiveRetainsRecallFloor(t *testing.T) {
	// A lone perfect prediction must not beat a supported operating point.
	scores := []float64{.99, .8, .7, .6, .5, .4, .3}
	labels := []float64{1, 0, 1, 1, 1, 0, 0}
	p := precisionPoint(scores, labels)
	if p.TP != 4 || p.FP != 1 || p.Recall != 1 || p.Precision != .8 {
		t.Fatalf("unexpected operating point: %+v", p)
	}
}
