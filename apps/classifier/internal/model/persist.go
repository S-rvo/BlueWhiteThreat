// apps/classifier/internal/model/persist.go
package model

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Model bundles everything needed to reconstruct a working classifier:
// the TF-IDF vectorizer's vocabulary/IDF weights, the logistic regression's
// weights/bias, the decision threshold used to turn a probability into a
// binary verdict, and a version string identifying the model artifact.
type Model struct {
	Vocabulary  map[string]int `json:"vocabulary"`
	IDF         []float64      `json:"idf"`
	Weights     []float64      `json:"weights"`
	Bias        float64        `json:"bias"`
	Threshold   float64        `json:"threshold"`
	Calibration Calibrator     `json:"calibration"`
	Version     string         `json:"version"`
}

// Save serializes m as JSON and writes it to path. The write is atomic:
// it writes to a temp file in the same directory as path and then renames
// it into place, so a crash or failed write never leaves a corrupted or
// truncated file at path.
func Save(path string, m *Model) error {
	if m == nil {
		return fmt.Errorf("model: save: model is nil")
	}

	data, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("model: marshal: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("model: create temp file for %s: %w", path, err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("model: write %s: %w", path, err)
	}

	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("model: close temp file for %s: %w", path, err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("model: rename %s: %w", path, err)
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
