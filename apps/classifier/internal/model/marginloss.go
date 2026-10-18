// apps/classifier/internal/model/marginloss.go
package model

import (
	"fmt"
	"math"
)

// TrainSquaredHinge minimizes weighted half-squared hinge loss plus L2/2
// via batch gradient descent with a fixed step of 0.1. Labels become -1/+1,
// and the bias is unregularized. Predict's sigmoid is then applied to the
// resulting margin for score-scale compatibility with LogisticRegression
// consumers; that score is not a calibrated probability. This objective is
// experimental, mirroring cmd/cvclassweights' squared-hinge OOF trainer so
// comparison tooling can exercise the same loss without duplicating its math.
func TrainSquaredHinge(x [][]float64, labels, weights []float64, epochs int, l2 float64) (*LogisticRegression, error) {
	return TrainMarginLoss(x, labels, weights, epochs, l2, false)
}

// TrainCappedHinge is TrainSquaredHinge with the loss below signed margin 0
// replaced by its tangent: L(m)=0.5-m for m<0, 0.5*(1-m)^2 for 0<=m<1, and 0
// otherwise, capping each example's derivative magnitude at 1 before
// weighting. This bounds the influence of badly misclassified examples
// without changing labels, mirroring cmd/cvclassweights' capped-hinge trainer.
func TrainCappedHinge(x [][]float64, labels, weights []float64, epochs int, l2 float64) (*LogisticRegression, error) {
	return TrainMarginLoss(x, labels, weights, epochs, l2, true)
}

// TrainMarginLoss is the shared implementation behind TrainSquaredHinge and
// TrainCappedHinge; see their docs for the loss definitions.
func TrainMarginLoss(x [][]float64, labels, weights []float64, epochs int, l2 float64, capped bool) (*LogisticRegression, error) {
	if len(x) == 0 || len(x) != len(labels) || len(x) != len(weights) || len(x[0]) == 0 {
		return nil, fmt.Errorf("margin loss needs nonempty aligned data and weights")
	}
	d := len(x[0])
	var total float64
	for i, row := range x {
		if len(row) != d || (labels[i] != 0 && labels[i] != 1) || weights[i] < 0 || math.IsNaN(weights[i]) || math.IsInf(weights[i], 0) {
			return nil, fmt.Errorf("invalid margin loss row")
		}
		for _, value := range row {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return nil, fmt.Errorf("nonfinite margin loss feature")
			}
		}
		total += weights[i]
	}
	if total <= 0 || math.IsInf(total, 0) || epochs < 1 || l2 < 0 || math.IsNaN(l2) || math.IsInf(l2, 0) {
		return nil, fmt.Errorf("invalid margin loss fitting parameters")
	}
	lr := &LogisticRegression{Weights: make([]float64, d)}
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
				return nil, fmt.Errorf("margin loss diverged")
			}
		}
		lr.Bias -= 0.1 * biasGradient / total
		if math.IsNaN(lr.Bias) || math.IsInf(lr.Bias, 0) {
			return nil, fmt.Errorf("margin loss bias diverged")
		}
	}
	return lr, nil
}
