// apps/classifier/internal/model/persist_test.go
package model

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	original := &Model{
		Vocabulary: map[string]int{"malware": 0, "phishing": 1, "cve": 2},
		IDF:        []float64{1.2, 0.8, 2.5},
		Weights:    []float64{0.5, -0.3, 1.1},
		Bias:       -0.42,
		Threshold:  0.5,
		Version:    "v1.0.0",
	}

	path := filepath.Join(t.TempDir(), "model.json")

	if err := Save(path, original); err != nil {
		t.Fatalf("Save() returned unexpected error: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if !reflect.DeepEqual(original, loaded) {
		t.Fatalf("round-tripped model = %+v, want %+v", loaded, original)
	}
}

func TestLoadNonexistentFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.json")

	m, err := Load(path)
	if err == nil {
		t.Fatal("Load() on nonexistent file returned nil error, want non-nil")
	}
	if m != nil {
		t.Fatalf("Load() on nonexistent file returned non-nil model: %+v", m)
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(path, []byte("not valid json{{{"), 0o644); err != nil {
		t.Fatalf("failed to set up test fixture: %v", err)
	}

	m, err := Load(path)
	if err == nil {
		t.Fatal("Load() on invalid JSON returned nil error, want non-nil")
	}
	if m != nil {
		t.Fatalf("Load() on invalid JSON returned non-nil model: %+v", m)
	}
}
