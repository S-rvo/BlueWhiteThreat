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
// binary verdict, a calibrator for turning scores into probabilities, and
// a version string identifying the model artifact.
type Model struct {
	Vocabulary  map[string]int     `json:"vocabulary"`
	IDF         []float64          `json:"idf"`
	Weights     []float64          `json:"weights"`
	Bias        float64            `json:"bias"`
	Threshold   float64            `json:"threshold"`
	Calibration Calibrator         `json:"calibration"`
	Version     string             `json:"version"`
	Validation  *ValidationSummary `json:"validation,omitempty"`
}

// ValidationSummary describes threshold selection, not independent proof of
// production quality. Legacy models omit this optional field.
type ValidationSummary struct {
	Documents       int     `json:"documents"`
	Precision       float64 `json:"precision"`
	Recall          float64 `json:"recall"`
	F1              float64 `json:"f1"`
	TargetPrecision float64 `json:"target_precision"`
	MinimumRecall   float64 `json:"minimum_recall"`
	ObjectiveMet    bool    `json:"objective_met"`
	SelectionRule   string  `json:"selection_rule"`
	LabelAuthority  string  `json:"label_authority"`
}

// Save serializes m as JSON and writes it to path. The write is atomic:
// it writes to a temp file in the same directory as path and then renames
// it into place, so a crash or failed write never leaves a corrupted or
// truncated file at path.
func Save(path string, m *Model) error {
	return save(path, m, false)
}

// SaveNew publishes a fully written artifact without replacing an existing
// file or symlink. A hard link makes publication atomic and exclusive.
func SaveNew(path string, m *Model) error {
	return save(path, m, true)
}

func save(path string, m *Model, exclusive bool) error {
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

	if exclusive {
		defer os.Remove(tmpPath)
		if err := os.Link(tmpPath, path); err != nil {
			return fmt.Errorf("model: publish new %s: %w", path, err)
		}
		return nil
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
