// apps/classifier/internal/model/calibration.go
package model

import "math"

// Calibrator rescales a raw probability (as returned by
// LogisticRegression.Predict) into a calibrated probability via Platt
// scaling: calibrated = sigmoid(A*logit(rawProb) + B). This matters because
// TrainWeighted is fit with BalancedWeights on an imbalanced dataset, which
// systematically distorts the raw sigmoid output away from the true
// positive rate at a given score — e.g. "0.9" out of the raw model might
// really only be right 40% of the time. Platt scaling corrects this using a
// held-out validation set's true labels, without touching the decision
// threshold (which is already chosen against the uncalibrated raw scores).
type Calibrator struct {
	A float64 `json:"a"`
	B float64 `json:"b"`
}

// Identity returns the no-op calibrator: Apply(p) == p for all p.
func Identity() Calibrator {
	return Calibrator{A: 1, B: 0}
}

// Apply maps rawProb through the calibration curve. A zero-value Calibrator
// (A == 0 && B == 0 — e.g. an older model.json saved before calibration was
// added) is treated as Identity rather than the degenerate constant
// sigmoid(0) = 0.5, so loading an old model never silently collapses every
// score to 0.5.
func (c Calibrator) Apply(rawProb float64) float64 {
	a, b := c.A, c.B
	if a == 0 && b == 0 {
		a = 1
	}
	return sigmoid(a*logit(rawProb) + b)
}

// logit is the inverse of sigmoid, clamped away from 0/1 so it stays finite.
func logit(p float64) float64 {
	const eps = 1e-12
	if p < eps {
		p = eps
	} else if p > 1-eps {
		p = 1 - eps
	}
	return math.Log(p / (1 - p))
}

// FitPlattScaling fits A and B by gradient descent, minimizing log loss of
// sigmoid(A*logit(rawScores[i])+B) against the true labels y. Intended to
// be called once per training run, on the same validation set used to pick
// the decision threshold — fitting on the training set itself would recover
// the same overconfidence the calibration is meant to correct.
func FitPlattScaling(rawScores []float64, y []float64, epochs int, learningRate float64) Calibrator {
	n := float64(len(rawScores))
	if n == 0 {
		return Identity()
	}

	zs := make([]float64, len(rawScores))
	for i, p := range rawScores {
		zs[i] = logit(p)
	}

	a, b := 1.0, 0.0
	for epoch := 0; epoch < epochs; epoch++ {
		var gradA, gradB float64
		for i, z := range zs {
			pred := sigmoid(a*z + b)
			err := pred - y[i]
			gradA += err * z
			gradB += err
		}
		a -= learningRate * gradA / n
		b -= learningRate * gradB / n
	}

	return Calibrator{A: a, B: b}
}
