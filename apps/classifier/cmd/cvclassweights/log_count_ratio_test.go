package main

import (
	"math"
	"reflect"
	"testing"
)

func TestLogCountRatiosPresenceAndClassSwap(t *testing.T) {
	x := [][]float64{{9, 0, 1}, {1, 0, 1}, {0, 3, 1}, {0, 1, 1}}
	r, err := fitLogCountRatios(x, []float64{1, 1, 0, 0})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(r[0]-math.Log(3)) > 1e-12 || math.Abs(r[1]+math.Log(3)) > 1e-12 || r[2] != 0 {
		t.Fatalf("ratios %v", r)
	}
	swapped, err := fitLogCountRatios(x, []float64{0, 0, 1, 1})
	if err != nil {
		t.Fatal(err)
	}
	for j := range r {
		if math.Abs(swapped[j]+r[j]) > 1e-12 {
			t.Fatal("class swap should reverse signs")
		}
	}
	input := []float64{2, 2, 5}
	got := applyLogCountRatios(input, r)
	if math.Abs(got[0]-1/math.Sqrt(2)) > 1e-12 || math.Abs(got[1]+1/math.Sqrt(2)) > 1e-12 || got[2] != 0 {
		t.Fatalf("transform %v", got)
	}
	if !reflect.DeepEqual(input, []float64{2, 2, 5}) {
		t.Fatal("input mutated")
	}
	if !reflect.DeepEqual(applyLogCountRatios([]float64{0, 0, 0}, r), []float64{0, 0, 0}) {
		t.Fatal("zero vector changed")
	}
}

func TestLogCountRatiosRejectInvalidTraining(t *testing.T) {
	for _, tc := range []struct {
		x [][]float64
		y []float64
	}{
		{nil, nil}, {[][]float64{{1}}, nil}, {[][]float64{{1}}, []float64{1}},
		{[][]float64{{1}, {1, 2}}, []float64{0, 1}}, {[][]float64{{1}, {1}}, []float64{0, 2}},
		{[][]float64{{math.NaN()}, {1}}, []float64{0, 1}}, {[][]float64{{-1}, {1}}, []float64{0, 1}},
	} {
		if _, err := fitLogCountRatios(tc.x, tc.y); err == nil {
			t.Fatal("invalid training accepted")
		}
	}
}
