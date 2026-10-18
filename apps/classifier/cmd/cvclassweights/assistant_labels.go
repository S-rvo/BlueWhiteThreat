package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
)

// Only fold-training labels may be replaced. Original labels still define
// groups, folds and OOF evaluation. This is an adaptive sensitivity experiment.
func loadAssistantLabels(path string, ids, texts []string, labels []float64) (map[string]float64, string, error) {
	result, _, hash, err := loadAssistantReview(path, ids, texts, labels)
	if err == nil && len(result) == 0 {
		return nil, "", fmt.Errorf("no assistant labels")
	}
	return result, hash, err
}

func loadAssistantReview(path string, ids, texts []string, labels []float64) (map[string]float64, map[string]bool, string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, "", err
	}
	var artifact struct {
		Documents []struct {
			ID             string          `json:"id"`
			Hash           string          `json:"text_sha256"`
			Original       int             `json:"original_human_label"`
			Label          json.RawMessage `json:"proposed_label"`
			Reviewer       string          `json:"reviewer"`
			HumanValidated bool            `json:"human_validated"`
		} `json:"documents"`
	}
	if err := json.Unmarshal(b, &artifact); err != nil {
		return nil, nil, "", err
	}
	indices := map[string]int{}
	for i, id := range ids {
		indices[id] = i
	}
	result := map[string]float64{}
	abstentions := map[string]bool{}
	seen := map[string]bool{}
	for _, p := range artifact.Documents {
		i, ok := indices[p.ID]
		if !ok || seen[p.ID] {
			return nil, nil, "", fmt.Errorf("assistant ID outside train or repeated: %s", p.ID)
		}
		seen[p.ID] = true
		if p.Hash != fmt.Sprintf("%x", sha256.Sum256([]byte(texts[i]))) || float64(p.Original) != labels[i] {
			return nil, nil, "", fmt.Errorf("assistant source text/label mismatch: %s", p.ID)
		}
		if p.Reviewer != "assistant" || p.HumanValidated {
			return nil, nil, "", fmt.Errorf("expected explicitly provisional assistant provenance")
		}
		if len(p.Label) == 0 {
			return nil, nil, "", fmt.Errorf("missing explicit proposed label: %s", p.ID)
		}
		var label *int
		if err := json.Unmarshal(p.Label, &label); err != nil {
			return nil, nil, "", err
		}
		if label == nil {
			abstentions[p.ID] = true
			continue
		}
		if *label != 0 && *label != 1 {
			return nil, nil, "", fmt.Errorf("invalid assistant label")
		}
		result[p.ID] = float64(*label)
	}
	return result, abstentions, fmt.Sprintf("%x", sha256.Sum256(b)), nil
}

// Exclude the whole duplicate group even when its representative was not reviewed.
func abstainedGroup(g group, abstentions map[string]bool) bool {
	if abstentions[g.id] {
		return true
	}
	for _, id := range g.members {
		if abstentions[id] {
			return true
		}
	}
	return false
}

func foldTrainingLabel(g group, proposals map[string]float64) float64 {
	if label, ok := proposals[g.id]; ok {
		return label
	}
	return g.label
}
