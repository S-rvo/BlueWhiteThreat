package main

import (
	"fmt"
	"math"
)

// fitLogCountRatios receives only the current fold's training vectors and labels.
// Counts are document presence, with one pseudo-count per class and feature.
// Class totals normalize feature distributions, rather than class prevalence.
func fitLogCountRatios(x [][]float64, labels []float64) ([]float64, error) {
	if len(x) == 0 || len(x) != len(labels) || len(x[0]) == 0 {
		return nil, fmt.Errorf("log-count-ratio requires nonempty aligned training data")
	}
	d := len(x[0])
	pos, neg := make([]float64, d), make([]float64, d)
	for j := range pos {
		pos[j], neg[j] = 1, 1
	}
	classes := [2]bool{}
	for i, row := range x {
		if len(row) != d || (labels[i] != 0 && labels[i] != 1) {
			return nil, fmt.Errorf("invalid log-count-ratio training row %d", i)
		}
		classes[int(labels[i])] = true
		for j, value := range row {
			if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
				return nil, fmt.Errorf("invalid log-count-ratio training feature")
			}
			if value > 0 {
				if labels[i] == 1 {
					pos[j]++
				} else {
					neg[j]++
				}
			}
		}
	}
	if !classes[0] || !classes[1] {
		return nil, fmt.Errorf("log-count-ratio needs both training classes")
	}
	var posTotal, negTotal float64
	for j := range pos {
		posTotal += pos[j]
		negTotal += neg[j]
	}
	ratios := make([]float64, d)
	for j := range ratios {
		ratios[j] = math.Log((pos[j] / posTotal) / (neg[j] / negTotal))
	}
	return ratios, nil
}

// applyLogCountRatios returns a fresh vector; cache and raw training rows stay intact.
func applyLogCountRatios(x, ratios []float64) []float64 {
	if len(x) != len(ratios) {
		panic("log-count-ratio dimension mismatch")
	}
	out := make([]float64, len(x))
	var squared float64
	for j := range x {
		out[j] = x[j] * ratios[j]
		squared += out[j] * out[j]
	}
	if squared > 0 {
		norm := math.Sqrt(squared)
		for j := range out {
			out[j] /= norm
		}
	}
	return out
}
