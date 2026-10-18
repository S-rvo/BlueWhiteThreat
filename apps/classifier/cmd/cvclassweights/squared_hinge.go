package main

import (
	"fmt"
	"math"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/model"
)

// trainSquaredHinge minimizes weighted half-squared hinge loss plus L2/2.
// Labels become -1/+1, the bias is unregularized, and the step is fixed at 0.1.
// Predict applies a sigmoid to the resulting margin for OOF output compatibility;
// that score is not a calibrated probability. This trainer is experimental only.
func trainSquaredHinge(x [][]float64, labels, weights []float64, epochs int, l2 float64) (*model.LogisticRegression, error) {
	return trainMarginLoss(x, labels, weights, epochs, l2, false)
}

// trainMarginLoss optionally replaces half-squared hinge below signed margin 0
// by its tangent: L(m)=0.5-m for m<0, 0.5*(1-m)^2 for 0<=m<1,
// and 0 otherwise. The derivative magnitude is capped at 1 before weighting.
// This tests bounded influence of misclassified examples, without changing labels.
func trainMarginLoss(x [][]float64, labels, weights []float64, epochs int, l2 float64, capped bool) (*model.LogisticRegression, error) {
	if len(x) == 0 || len(x) != len(labels) || len(x) != len(weights) || len(x[0]) == 0 {
		return nil, fmt.Errorf("squared hinge needs nonempty aligned data and weights")
	}
	d := len(x[0])
	var total float64
	for i, row := range x {
		if len(row) != d || (labels[i] != 0 && labels[i] != 1) || weights[i] < 0 || math.IsNaN(weights[i]) || math.IsInf(weights[i], 0) {
			return nil, fmt.Errorf("invalid squared hinge row")
		}
		for _, value := range row {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return nil, fmt.Errorf("nonfinite squared hinge feature")
			}
		}
		total += weights[i]
	}
	if total <= 0 || math.IsInf(total, 0) || epochs < 1 || l2 < 0 || math.IsNaN(l2) || math.IsInf(l2, 0) {
		return nil, fmt.Errorf("invalid squared hinge fitting parameters")
	}
	lr := &model.LogisticRegression{Weights: make([]float64, d)}
	gradient := make([]float64, d)
	for epoch := 0; epoch < epochs; epoch++ {
		clear(gradient)
		var biasGradient float64
		for i, row := range x {
			margin := lr.Bias
			for j, value := range row {
				margin += lr.Weights[j] * value
			}
			sign := 2*labels[i] - 1
			violation := 1 - sign*margin
			if violation <= 0 {
				continue
			}
			if capped && violation > 1 {
				violation = 1
			}
			derivative := -sign * violation * weights[i]
			for j, value := range row {
				gradient[j] += derivative * value
			}
			biasGradient += derivative
		}
		for j := range lr.Weights {
			lr.Weights[j] -= 0.1 * (gradient[j]/total + l2*lr.Weights[j])
			if math.IsNaN(lr.Weights[j]) || math.IsInf(lr.Weights[j], 0) {
				return nil, fmt.Errorf("squared hinge diverged")
			}
		}
		lr.Bias -= 0.1 * biasGradient / total
		if math.IsNaN(lr.Bias) || math.IsInf(lr.Bias, 0) {
			return nil, fmt.Errorf("squared hinge bias diverged")
		}
	}
	return lr, nil
}

// fitObjective is shared by OOF fitting and full-train artifact fitting so the
// deployable transform and training objective cannot silently diverge.
func fitObjective(x [][]float64, labels, weights []float64, loss string) (*model.LogisticRegression, error) {
	switch loss {
	case "logistic":
		return model.TrainWeighted(x, labels, weights, epochs, learningRate, l2), nil
	case "squared-hinge":
		return trainSquaredHinge(x, labels, weights, epochs, l2)
	case "capped-hinge":
		return trainMarginLoss(x, labels, weights, epochs, l2, true)
	default:
		return nil, fmt.Errorf("unknown training loss %q", loss)
	}
}
