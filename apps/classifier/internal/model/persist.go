// apps/classifier/internal/model/persist.go
package model

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// Model bundles everything needed to reconstruct a working classifier:
// the TF-IDF vectorizer's vocabulary/IDF weights, the logistic regression's
// weights/bias, the decision threshold used to turn a probability into a
// binary verdict, a calibrator for turning scores into probabilities, and
// a version string identifying the model artifact.
type Model struct {
	// A present char_ngrams field identifies the historical vectorizer format.
	CharNgrams  *bool          `json:"char_ngrams,omitempty"`
	SublinearTF bool           `json:"sublinear_tf,omitempty"`
	Vocabulary  map[string]int `json:"vocabulary"`
	IDF         []float64      `json:"idf"`
	Weights     []float64      `json:"weights"`
	Bias        float64        `json:"bias"`
	Threshold   float64        `json:"threshold"`
	// SourceThresholds optionally specializes the decision boundary for known
	// ingestion sources. Unknown or empty sources use Threshold.
	SourceThresholds map[string]float64 `json:"source_thresholds,omitempty"`
	// SourceHeads optionally replaces the shared logistic head for a known
	// source while retaining the common frozen vectorizer.
	SourceHeads map[string]SourceHead `json:"source_heads,omitempty"`
	Calibration Calibrator            `json:"calibration"`
	Version     string                `json:"version"`
	Validation  *ValidationSummary    `json:"validation,omitempty"`
}

type SourceHead struct {
	Weights     []float64  `json:"weights"`
	Bias        float64    `json:"bias"`
	Calibration Calibrator `json:"calibration"`
}

// ValidationSummary describes threshold selection, not independent proof of
// production quality. Legacy models omit this optional field.
type ValidationSummary struct {
	Documents          int     `json:"documents"`
	Precision          float64 `json:"precision"`
	Recall             float64 `json:"recall"`
	F1                 float64 `json:"f1"`
	TargetPrecision    float64 `json:"target_precision"`
	MinimumRecall      float64 `json:"minimum_recall"`
	ObjectiveMet       bool    `json:"objective_met"`
	SelectionRule      string  `json:"selection_rule"`
	LabelAuthority     string  `json:"label_authority"`
	EffectivePrecision float64 `json:"effective_precision,omitempty"`
	EffectiveRecall    float64 `json:"effective_recall,omitempty"`
	EffectiveF1        float64 `json:"effective_f1,omitempty"`
	PromotionEligible  *bool   `json:"promotion_eligible,omitempty"`
}

// Validate rejects artifacts whose vectorizer, heads or decision policy cannot
// be combined coherently.
func (m *Model) Validate() error {
	if m == nil {
		return fmt.Errorf("model is nil")
	}
	dimension := len(m.Vocabulary)
	if len(m.IDF) != dimension || len(m.Weights) != dimension {
		return fmt.Errorf("dimension mismatch: vocabulary=%d idf=%d weights=%d", dimension, len(m.IDF), len(m.Weights))
	}
	seen := make([]bool, dimension)
	for token, index := range m.Vocabulary {
		if index < 0 || index >= dimension || seen[index] {
			return fmt.Errorf("invalid vocabulary index %d for %q", index, token)
		}
		seen[index] = true
	}
	if !finite(m.Bias) || !probability(m.Threshold) || !validCalibrator(m.Calibration) {
		return fmt.Errorf("invalid shared bias, threshold or calibration")
	}
	for source, threshold := range m.SourceThresholds {
		if source == "" || !probability(threshold) {
			return fmt.Errorf("invalid threshold for source %q", source)
		}
	}
	for source, head := range m.SourceHeads {
		if source == "" || len(head.Weights) != dimension {
			return fmt.Errorf("invalid head dimension for source %q: got %d, want %d", source, len(head.Weights), dimension)
		}
		if !finite(head.Bias) || !validCalibrator(head.Calibration) {
			return fmt.Errorf("invalid head bias or calibration for source %q", source)
		}
		for _, value := range head.Weights {
			if !finite(value) {
				return fmt.Errorf("head %q contains a non-finite coefficient", source)
			}
		}
	}
	for _, values := range [][]float64{m.IDF, m.Weights} {
		for _, value := range values {
			if !finite(value) {
				return fmt.Errorf("model contains a non-finite coefficient")
			}
		}
	}
	return nil
}

func finite(value float64) bool         { return !math.IsNaN(value) && !math.IsInf(value, 0) }
func probability(value float64) bool    { return finite(value) && value >= 0 && value <= 1 }
func validCalibrator(c Calibrator) bool { return finite(c.A) && finite(c.B) }

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
	if err := m.Validate(); err != nil {
		return fmt.Errorf("model: validate: %w", err)
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
	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("model: validate %s: %w", path, err)
	}

	return &m, nil
}
