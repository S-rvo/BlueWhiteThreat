// Package experimentallexical connects Go to a private Python candidate.
// It is deliberately not wired into production classification.
package experimentallexical

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
)

type Document struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Source string `json:"source"`
}
type Result struct {
	ID        string   `json:"id"`
	Score     float64  `json:"score"`
	Relevant  *bool    `json:"relevant"`
	Threshold *float64 `json:"threshold"`
	Status    string   `json:"status"`
	Version   string   `json:"version"`
}
type Bridge struct{ Python, Script, Candidate string }

func (b Bridge) Score(ctx context.Context, docs []Document) ([]Result, error) {
	if len(docs) == 0 {
		return []Result{}, nil
	}
	if b.Python == "" || b.Script == "" || b.Candidate == "" {
		return nil, fmt.Errorf("missing bridge configuration")
	}
	var input bytes.Buffer
	for _, doc := range docs {
		if err := json.NewEncoder(&input).Encode(doc); err != nil {
			return nil, err
		}
	}
	command := exec.CommandContext(ctx, b.Python, b.Script, "--candidate", b.Candidate)
	command.Stdin = &input
	var output, stderr bytes.Buffer
	command.Stdout = &output
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("experimental scorer failed: %w; %s", err, stderr.String())
	}
	decoder := json.NewDecoder(&output)
	results := make([]Result, 0, len(docs))
	for decoder.More() {
		var r Result
		if err := decoder.Decode(&r); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	if len(results) != len(docs) {
		return nil, fmt.Errorf("result count mismatch")
	}
	for i, r := range results {
		if r.ID != docs[i].ID || math.IsNaN(r.Score) || math.IsInf(r.Score, 0) || r.Score < 0 || r.Score > 1 || r.Version == "" {
			return nil, fmt.Errorf("invalid result at %d", i)
		}
		switch r.Status {
		case "scored":
			if r.Relevant == nil || r.Threshold == nil || math.IsNaN(*r.Threshold) || math.IsInf(*r.Threshold, 0) || *r.Threshold < 0 || *r.Threshold > 1 || *r.Relevant != (r.Score >= *r.Threshold) {
				return nil, fmt.Errorf("invalid scored decision at %d", i)
			}
		case "unknown_source_abstention":
			if r.Relevant != nil || r.Threshold != nil {
				return nil, fmt.Errorf("abstention contains verdict")
			}
		default:
			return nil, fmt.Errorf("unknown scorer status")
		}
	}
	return results, nil
}
