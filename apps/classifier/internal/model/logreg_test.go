// apps/classifier/internal/model/logreg_test.go
package model

import (
	"math"
	"testing"
)

func TestSigmoid(t *testing.T) {
	tests := []struct {
		name string
		z    float64
		want float64
		tol  float64
	}{
		{"zero", 0, 0.5, 1e-9},
		{"large positive", 50, 1.0, 1e-9},
		{"large negative", -50, 0.0, 1e-9},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sigmoid(tt.z)
			if math.Abs(got-tt.want) > tt.tol {
				t.Fatalf("sigmoid(%v) = %v, want %v (tol %v)", tt.z, got, tt.want, tt.tol)
			}
		})
	}
}

func TestTrainConvergesOnLinearlySeparableData(t *testing.T) {
	X := [][]float64{{0}, {0}, {1}, {4}, {5}, {5}}
	y := []float64{0, 0, 0, 1, 1, 1}

	lr := Train(X, y, 2000, 0.1, 0.0)

	if got := lr.Predict([]float64{0}); got >= 0.5 {
		t.Fatalf("Predict([0]) = %v, want < 0.5", got)
	}
	if got := lr.Predict([]float64{5}); got < 0.5 {
		t.Fatalf("Predict([5]) = %v, want >= 0.5", got)
	}
}

func TestPredictOnZeroValueIsPure(t *testing.T) {
	lr := &LogisticRegression{}

	tests := [][]float64{
		{},
		{0, 0, 0},
		{1, 2, 3},
		{-5, 10, -3.5},
	}

	for _, x := range tests {
		got := lr.Predict(x)
		if math.Abs(got-0.5) > 1e-9 {
			t.Fatalf("Predict(%v) on zero-value LogisticRegression = %v, want 0.5", x, got)
		}
	}
}
