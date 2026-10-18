package main

import (
	"math"
	"strings"
)

// featureFamilies records namespaces in the already fold-fitted vocabulary.
// Tokenize never emits underscores, so non-prefixed underscore terms are pairs.
func featureFamilies(vocabulary map[string]int) []int {
	families := make([]int, len(vocabulary))
	for term, index := range vocabulary {
		switch {
		case strings.HasPrefix(term, "c_"):
			families[index] = 2
		case strings.HasPrefix(term, "s_"):
			families[index] = 3
		case strings.Contains(term, "_"):
			families[index] = 1
		}
	}
	return families
}

// normalizeFamilies gives each nonzero feature namespace equal squared norm,
// followed by overall L2 normalization. Empty families stay zero. This uses
// no labels or batch statistics and never changes the vocabulary or input.
func normalizeFamilies(x []float64, families []int) []float64 {
	if len(x) != len(families) {
		panic("feature family dimension mismatch")
	}
	var sums [4]float64
	for i, value := range x {
		sums[families[i]] += value * value
	}
	active := 0
	for _, sum := range sums {
		if sum > 0 {
			active++
		}
	}
	out := make([]float64, len(x))
	if active == 0 {
		return out
	}
	for i, value := range x {
		if sums[families[i]] > 0 {
			out[i] = value / math.Sqrt(sums[families[i]]*float64(active))
		}
	}
	return out
}
