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
//
// If len(x) != len(Weights), only the overlapping prefix is used; this is
// intentional (no panic, no error) so that Predict stays safe to call on a
// zero-value LogisticRegression{}. Callers are responsible for ensuring x's
// dimensionality matches Weights before calling Predict — e.g. a future
// Classifier orchestrator should validate len(vectorizer.Vocabulary) ==
// len(model.Weights) at load time rather than relying on Predict to catch
// a mismatch.
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
//
// Train is a thin wrapper over TrainWeighted with uniform per-example
// weights — every example counts equally toward the loss, which biases the
// learned model toward the majority class on an imbalanced dataset. Callers
// training on imbalanced data should use TrainWeighted with BalancedWeights
// instead.
func Train(X [][]float64, y []float64, epochs int, learningRate, l2 float64) *LogisticRegression {
	return TrainWeighted(X, y, nil, epochs, learningRate, l2)
}

// BalancedWeights returns a per-example weight for each label in y such
// that each class contributes equally to the total weighted loss, following
// the standard "balanced" scheme (weight[i] = n / (numClasses * count[y[i]])).
// On an imbalanced dataset (e.g. 8% positives), this makes a single positive
// example count for roughly as much as a single negative example in
// aggregate, countering the gradient's natural bias toward the majority
// class — without this, the majority class dominates the unweighted loss
// and the model learns to under-predict the minority class (low recall).
func BalancedWeights(y []float64) []float64 {
	counts := map[float64]float64{}
	for _, label := range y {
		counts[label]++
	}
	n := float64(len(y))
	numClasses := float64(len(counts))

	weights := make([]float64, len(y))
	for i, label := range y {
		weights[i] = n / (numClasses * counts[label])
	}
	return weights
}

// TrainWeighted is Train with an optional per-example weight applied to the
// loss gradient. A nil weights slice is equivalent to passing all 1s (same
// behavior as Train). len(weights) must equal len(X) when non-nil.
func TrainWeighted(X [][]float64, y []float64, weights []float64, epochs int, learningRate, l2 float64) *LogisticRegression {
	numFeatures := 0
	if len(X) > 0 {
		numFeatures = len(X[0])
	}

	lr := &LogisticRegression{
		Weights: make([]float64, numFeatures),
		Bias:    0,
	}

	if len(X) == 0 {
		return lr
	}

	if weights == nil {
		weights = make([]float64, len(X))
		for i := range weights {
			weights[i] = 1
		}
	}

	var sumWeights float64
	for _, w := range weights {
		sumWeights += w
	}
	if sumWeights == 0 {
		return lr
	}

	// TF-IDF vectors are sparse. Cache nonzero coordinates once rather than
	// visiting thousands of zero features on every optimization step.
	type feature struct {
		index int
		value float64
	}
	sparse := make([][]feature, len(X))
	for i, row := range X {
		for j, value := range row {
			if value != 0 {
				sparse[i] = append(sparse[i], feature{j, value})
			}
		}
	}
	weightGrad := make([]float64, numFeatures)

	for epoch := 0; epoch < epochs; epoch++ {
		for i := range weightGrad {
			weightGrad[i] = 0
		}
		var biasGrad float64

		for i, row := range sparse {
			z := lr.Bias
			for _, f := range row {
				z += lr.Weights[f.index] * f.value
			}
			pred := sigmoid(z)
			err := (pred - y[i]) * weights[i]
			for _, f := range row {
				weightGrad[f.index] += err * f.value
			}
			biasGrad += err
		}

		for j := range lr.Weights {
			grad := weightGrad[j]/sumWeights + l2*lr.Weights[j]
			lr.Weights[j] -= learningRate * grad
		}
		lr.Bias -= learningRate * (biasGrad / sumWeights)
	}

	return lr
}
