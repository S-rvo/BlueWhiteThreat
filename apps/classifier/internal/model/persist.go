// apps/classifier/internal/model/persist.go
package model

import (
	"encoding/json"
	"fmt"
	"os"
)

// Model bundles everything needed to reconstruct a working classifier:
// the TF-IDF vectorizer's vocabulary/IDF weights, the logistic regression's
// weights/bias, the decision threshold used to turn a probability into a
// binary verdict, and a version string identifying the model artifact.
type Model struct {
	Vocabulary map[string]int `json:"vocabulary"`
	IDF        []float64      `json:"idf"`
	Weights    []float64      `json:"weights"`
	Bias       float64        `json:"bias"`
	Threshold  float64        `json:"threshold"`
	Version    string         `json:"version"`
}

// Save serializes m as JSON and writes it to path, creating or
// truncating the file as needed.
func Save(path string, m *Model) error {
	data, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("model: marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("model: write %s: %w", path, err)
	}
	return nil
}

// Load reads path and deserializes it into a *Model. Returns an error
// if the file does not exist or contains invalid JSON.
func Load(path string) (*Model, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("model: read %s: %w", path, err)
	}

	var m Model
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("model: unmarshal %s: %w", path, err)
	}

	return &m, nil
}
