package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/model"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/tfidf"
)

func fitFullTrainCandidate(groups []group, minDF int, version string) (*model.Model, error) {
	return fitFullTrainCandidateWithLoss(groups, minDF, version, "logistic")
}

func fitFullTrainCandidateWithLoss(groups []group, minDF int, version, loss string) (*model.Model, error) {
	texts := make([]string, len(groups))
	labels := make([]float64, len(groups))
	classes := [2]int{}
	for i, g := range groups {
		if g.label != 0 && g.label != 1 {
			return nil, fmt.Errorf("invalid candidate label")
		}
		texts[i], labels[i] = g.text, g.label
		classes[int(g.label)]++
	}
	if classes[0] == 0 || classes[1] == 0 {
		return nil, fmt.Errorf("candidate training needs both classes")
	}
	v, err := tfidf.FitWithMinDocumentFrequency(texts, labels, minDF)
	if err != nil {
		return nil, err
	}
	if len(v.Vocabulary) == 0 {
		return nil, fmt.Errorf("candidate has no retained features")
	}
	x := make([][]float64, len(texts))
	for i, t := range texts {
		x[i] = v.Transform(t)
	}
	lr, err := fitObjective(x, labels, model.BalancedWeights(labels), loss)
	if err != nil {
		return nil, err
	}
	return &model.Model{Vocabulary: v.Vocabulary, IDF: v.IDF, Weights: lr.Weights, Bias: lr.Bias, Threshold: cutoff, Calibration: model.Identity(), Version: version}, nil
}

func saveFullTrainCandidate(out string, groups []group, minDF int, loss string) error {
	protocol, err := os.ReadFile(filepath.Join(out, "protocol.json"))
	if err != nil {
		return err
	}
	protocolSHA := fmt.Sprintf("%x", sha256.Sum256(protocol))
	version := "provisional-human-mindf-" + fmt.Sprint(minDF) + "-" + protocolSHA[:16]
	if loss != "logistic" {
		version = "provisional-human-" + loss + "-mindf-" + fmt.Sprint(minDF) + "-" + protocolSHA[:16]
	}
	candidate, err := fitFullTrainCandidateWithLoss(groups, minDF, version, loss)
	if err != nil {
		return err
	}
	path := filepath.Join(out, "candidate.json")
	if err = model.SaveNew(path, candidate); err != nil {
		return err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	ids := make([]string, len(groups))
	for i, g := range groups {
		ids[i] = g.id
	}
	manifest := map[string]any{"model_sha256": fmt.Sprintf("%x", sha256.Sum256(raw)), "protocol_sha256": protocolSHA, "version": version, "training_representative_ids": ids, "training_groups": len(groups), "vocabulary_size": len(candidate.Vocabulary), "threshold": cutoff, "calibration": "identity; not probability-calibrated", "label_authority": "reviewed labels restricted to frozen human train IDs", "status": "provisional; adaptive train OOF diagnostics only; independent confirmation absent", "production_model_modified": false}
	manifest["loss"] = loss
	manifest["score_semantics"] = "uncalibrated sigmoid of learned linear score; not probability evidence"
	manifest["threshold_selection"] = "fixed experimental 0.5; no validation tuning or precision guarantee"
	return writeNew(filepath.Join(out, "candidate-manifest.json"), manifest)
}
