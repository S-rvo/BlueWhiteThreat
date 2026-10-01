// apps/classifier/internal/model/logreg.go
package model

import "math"

// LogisticRegression is a binary classifier over fixed-length feature
// vectors (e.g. TF-IDF vectors produced by internal/tfidf), parameterized
// by a weight per feature and a scalar bias.
type LogisticRegression struct {
	Weights []float64
	Bias    float64
}

// sigmoid is the standard logistic function 1 / (1 + e^-z).
func sigmoid(z float64) float64 {
	return 1 / (1 + math.Exp(-z))
}

// Predict returns the sigmoid-activated probability that x belongs to the
// positive class, given the current Weights/Bias.
func (lr *LogisticRegression) Predict(x []float64) float64 {
	z := lr.Bias
	for i, w := range lr.Weights {
		if i >= len(x) {
			break
		}
		z += w * x[i]
	}
	return sigmoid(z)
}

// Train fits Weights and Bias via batch gradient descent on the full
// dataset X/y for the given number of epochs, with the given learning
// rate and L2 regularization strength applied to Weights only (not Bias).
// Returns a new *LogisticRegression with learned Weights (initialized to
// zero vector of len(X[0])) and Bias (initialized to 0).
func Train(X [][]float64, y []float64, epochs int, learningRate, l2 float64) *LogisticRegression {
	numFeatures := 0
	if len(X) > 0 {
		numFeatures = len(X[0])
	}

	lr := &LogisticRegression{
		Weights: make([]float64, numFeatures),
		Bias:    0,
	}

	n := float64(len(X))
	if n == 0 {
		return lr
	}

	weightGrad := make([]float64, numFeatures)

	for epoch := 0; epoch < epochs; epoch++ {
		for i := range weightGrad {
			weightGrad[i] = 0
		}
		var biasGrad float64

		for i, x := range X {
			pred := lr.Predict(x)
			err := pred - y[i]
			for j, xj := range x {
				weightGrad[j] += err * xj
			}
			biasGrad += err
		}

		for j := range lr.Weights {
			grad := weightGrad[j]/n + l2*lr.Weights[j]
			lr.Weights[j] -= learningRate * grad
		}
		lr.Bias -= learningRate * (biasGrad / n)
	}

	return lr
}
