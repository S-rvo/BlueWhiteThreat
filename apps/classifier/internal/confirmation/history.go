package confirmation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type receipt struct {
	ModelSHA256 string        `json:"model_sha256"`
	BatchSHA256 string        `json:"batch_sha256"`
	Documents   []fingerprint `json:"documents"`
}

// Sibling campaigns share a history. Changing candidate or renaming the batch
// does not make previously scored documents eligible again. This is a local
// protocol guard, not protection against deliberate deletion of the history.
func reserve(dir string, docs []Document, result Result) error {
	history := filepath.Join(filepath.Dir(dir), ".confirmation-history")
	if err := os.MkdirAll(history, 0700); err != nil {
		return err
	}
	lock := filepath.Join(history, "lock")
	if err := writeExclusive(lock, []byte("reservation in progress\n")); err != nil {
		return fmt.Errorf("confirmation history locked; audit stale locks rather than rerunning: %w", err)
	}
	defer func() { _ = os.Remove(lock) }()
	paths, err := filepath.Glob(filepath.Join(history, "*.json"))
	if err != nil {
		return err
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var prior receipt
		if err = json.Unmarshal(data, &prior); err != nil {
			return err
		}
		if len(prior.Documents) == 0 {
			return fmt.Errorf("invalid confirmation history record %s", path)
		}
		for _, d := range docs {
			if reason := collision(printOf(d), prior.Documents); reason != "" {
				return fmt.Errorf("confirmation document %s was already consumed by another campaign: %s", d.ID, reason)
			}
		}
	}
	if err = writeJSON(filepath.Join(dir, "attempt.json"), result); err != nil {
		return fmt.Errorf("campaign may already be consumed: %w", err)
	}
	entry := receipt{result.ModelSHA256, result.BatchSHA256, prints(docs)}
	return writeJSON(filepath.Join(history, result.BatchSHA256+".json"), entry)
}
