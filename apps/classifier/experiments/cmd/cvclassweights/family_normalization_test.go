package main

import (
	"math"
	"reflect"
	"testing"
)

func TestFamilyNormalizationBalancesNamespacesAndPreservesInput(t *testing.T) {
	v := map[string]int{"word": 0, "word_pair": 1, "c_test": 2, "c_other": 3, "s_cve_mention": 4}
	families := featureFamilies(v)
	if !reflect.DeepEqual(families, []int{0, 1, 2, 2, 3}) {
		t.Fatal(families)
	}
	x := []float64{3, 2, 30, 40, 1}
	out := normalizeFamilies(x, families)
	var mass [4]float64
	for i, value := range out {
		mass[families[i]] += value * value
	}
	for _, value := range mass {
		if math.Abs(value-.25) > 1e-12 {
			t.Fatal(mass)
		}
	}
	if math.Abs(out[2]/out[3]-.75) > 1e-12 {
		t.Fatal("within-family ratios changed")
	}
	if !reflect.DeepEqual(x, []float64{3, 2, 30, 40, 1}) {
		t.Fatal("input changed")
	}
	// Scaling a single namespace leaves the output unchanged.
	x[2], x[3] = 300, 400
	other := normalizeFamilies(x, families)
	for i := range out {
		if math.Abs(out[i]-other[i]) > 1e-12 {
			t.Fatal("namespace scale affected output")
		}
	}
}

func TestFamilyNormalizationHandlesMissingNamespaces(t *testing.T) {
	if !reflect.DeepEqual(normalizeFamilies([]float64{0, 0}, []int{0, 2}), []float64{0, 0}) {
		t.Fatal("zero vector changed")
	}
	out := normalizeFamilies([]float64{3, 4, 0}, []int{2, 2, 0})
	if math.Abs(out[0]-.6) > 1e-12 || math.Abs(out[1]-.8) > 1e-12 || out[2] != 0 {
		t.Fatal(out)
	}
}
