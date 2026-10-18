// cvocabulary compares word-vocabulary minimum document frequencies with
// grouped out-of-fold predictions from human-reviewed training examples only.
// It never selects a production model or consumes the confirmation campaign.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"sort"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/confirmation"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/dedup"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/evalmetrics"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/model"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/overlap"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/review"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/tfidf"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/vocabexperiment"
)

const (
	folds        = 5
	epochs       = 1200
	learningRate = 2.0
	decisionCut  = 0.5
)

type trainProtocol struct {
	TrainIDs []string `json:"train_ids"`
}

type testRow struct {
	ID     string             `json:"id"`
	Label  float64            `json:"label"`
	Fold   int                `json:"fold"`
	Scores map[string]float64 `json:"scores_by_candidate"`
}

type candidateSpec struct {
	Name  string
	MinDF int
	Mode  string
	L2    float64
}

type metrics struct {
	N, TP, FP, TN, FN                                        int
	Precision, Recall, F1, Accuracy, AveragePrecision, Brier float64
}

func hashFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(b)), nil
}

func writeJSON(path string, value any) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		return err
	}
	return f.Sync()
}

func averagePrecision(scores, labels []float64) float64 {
	order := make([]int, len(scores))
	positives := 0
	for i := range order {
		order[i] = i
		if labels[i] == 1 {
			positives++
		}
	}
	if positives == 0 {
		return 0
	}
	sort.SliceStable(order, func(i, j int) bool { return scores[order[i]] > scores[order[j]] })
	hits, sum := 0, 0.0
	for rank, idx := range order {
		if labels[idx] == 1 {
			hits++
			sum += float64(hits) / float64(rank+1)
		}
	}
	return sum / float64(positives)
}

func summarize(scores, labels []float64) metrics {
	tp, fp, tn, fn := evalmetrics.ConfusionCounts(scores, labels, decisionCut)
	a, p, r, f := evalmetrics.ConfusionAt(scores, labels, decisionCut)
	brier := 0.0
	for i, s := range scores {
		brier += (s - labels[i]) * (s - labels[i])
	}
	if len(scores) > 0 {
		brier /= float64(len(scores))
	}
	return metrics{len(scores), tp, fp, tn, fn, p, r, f, a, averagePrecision(scores, labels), brier}
}

func groupedTraining(ids, texts []string, labels []float64) ([]dedup.Group, int, int) {
	exact, exactConflicts := dedup.GroupByNormalizedText(ids, texts, labels)
	prints := make([]overlap.Print, len(exact))
	for i, g := range exact {
		prints[i] = overlap.Fingerprint(g.Texts[0])
	}
	parent := make([]int, len(exact))
	for i := range parent {
		parent[i] = i
	}
	var root func(int) int
	root = func(i int) int {
		if parent[i] != i {
			parent[i] = root(parent[i])
		}
		return parent[i]
	}
	for i := range exact {
		for j := i + 1; j < len(exact); j++ {
			if overlap.Similar(prints[i], prints[j]) {
				a, b := root(i), root(j)
				if a != b {
					parent[b] = a
				}
			}
		}
	}
	clusters := map[int][]int{}
	var order []int
	for i := range exact {
		r := root(i)
		if _, ok := clusters[r]; !ok {
			order = append(order, r)
		}
		clusters[r] = append(clusters[r], i)
	}
	groups := make([]dedup.Group, 0, len(order))
	conflicts := 0
	for _, r := range order {
		members := clusters[r]
		label := exact[members[0]].Label
		consistent := true
		group := dedup.Group{Key: exact[members[0]].Key, Label: label}
		for _, idx := range members {
			if exact[idx].Label != label {
				consistent = false
			}
			group.IDs = append(group.IDs, exact[idx].IDs...)
			group.Texts = append(group.Texts, exact[idx].Texts...)
		}
		if !consistent {
			conflicts++
			continue
		}
		groups = append(groups, group)
	}
	return groups, len(exactConflicts), conflicts
}

func run() error {
	out := flag.String("out", "data/cvocabulary-min-df-20261013", "new output directory")
	flag.Parse()
	campaignDir := os.Getenv("CONFIRMATION_DIR")
	if campaignDir == "" {
		campaignDir = "data/confirmation-campaign"
	}
	campaign, err := confirmation.Load(campaignDir, ".")
	if err != nil {
		return err
	}
	reviewedLabels, err := review.LoadReviewed("data/reviewed_labels.csv")
	if err != nil {
		return err
	}
	reviewed := map[string]int{}
	for id, l := range reviewedLabels {
		reviewed[id] = l.Label
	}
	protocolBytes, err := os.ReadFile("data/score-investigation-20261013/protocol.json")
	if err != nil {
		return err
	}
	var protocol trainProtocol
	if err := json.Unmarshal(protocolBytes, &protocol); err != nil {
		return err
	}
	textsByID := map[string]string{}
	for _, d := range campaign.Corpus {
		textsByID[d.ID] = d.Text
	}
	var ids, texts []string
	var labels []float64
	for _, id := range protocol.TrainIDs {
		label, ok := reviewed[id]
		if !ok {
			continue
		}
		text, exists := textsByID[id]
		if !exists {
			return fmt.Errorf("training id %s absent from frozen corpus", id)
		}
		ids = append(ids, id)
		texts = append(texts, text)
		labels = append(labels, float64(label))
	}
	if len(ids) != 573 {
		return fmt.Errorf("expected 573 reviewed training examples, got %d", len(ids))
	}
	groups, exactConflictCount, nearConflictCount := groupedTraining(ids, texts, labels)
	// Conflicting exact and near-duplicate groups are excluded rather than
	// assigned an arbitrary label. One representative from each consistent
	// cluster becomes the atomic unit in cross-validation.
	byClass := [2][]int{}
	for i, g := range groups {
		if g.Label != 0 && g.Label != 1 {
			return fmt.Errorf("unexpected label %.1f", g.Label)
		}
		byClass[int(g.Label)] = append(byClass[int(g.Label)], i)
	}
	foldMembers := make([][]int, folds)
	rng := rand.New(rand.NewSource(20261013))
	for label := range byClass {
		rng.Shuffle(len(byClass[label]), func(i, j int) {
			byClass[label][i], byClass[label][j] = byClass[label][j], byClass[label][i]
		})
		for i, idx := range byClass[label] {
			foldMembers[i%folds] = append(foldMembers[i%folds], idx)
		}
	}
	assign := make([]int, len(groups))
	for fold, members := range foldMembers {
		for _, idx := range members {
			assign[idx] = fold
		}
	}
	featureCandidates := []candidateSpec{
		{Name: "mixed_base", Mode: "mixed"},
		{Name: "word_unigrams_min_df_2", MinDF: 2, Mode: "words"},
		{Name: "word_bigrams_min_df_2", MinDF: 2, Mode: "bigrams"},
		{Name: "word_unigrams_min_df_5", MinDF: 5, Mode: "words"},
		{Name: "word_unigrams_min_df_10", MinDF: 10, Mode: "words"},
	}
	l2Grid := []float64{0.5, 0.05, 0.005}
	candidates := make([]candidateSpec, 0, len(featureCandidates)*len(l2Grid))
	for _, feature := range featureCandidates {
		for _, l2 := range l2Grid {
			candidate := feature
			candidate.L2 = l2
			candidate.Name = fmt.Sprintf("%s_l2_%g", feature.Name, l2)
			candidates = append(candidates, candidate)
		}
	}
	rows := make([]testRow, len(groups))
	labelsAll := make([]float64, len(groups))
	for i, g := range groups {
		rows[i] = testRow{ID: g.IDs[0], Label: g.Label, Fold: assign[i], Scores: map[string]float64{}}
		labelsAll[i] = g.Label
	}
	for fold := 0; fold < folds; fold++ {
		var trainTexts []string
		var trainY []float64
		var testIdx []int
		for i, g := range groups {
			if assign[i] == fold {
				testIdx = append(testIdx, i)
			} else {
				trainTexts = append(trainTexts, g.Texts[0])
				trainY = append(trainY, g.Label)
			}
		}
		if len(testIdx) == 0 || len(trainY) == 0 {
			return fmt.Errorf("fold %d has no train or test rows", fold)
		}
		for _, candidate := range candidates {
			var v *tfidf.Vectorizer
			switch candidate.Mode {
			case "mixed":
				v = tfidf.Fit(trainTexts, trainY)
			case "words":
				v = vocabexperiment.FitMinDF(trainTexts, false, candidate.MinDF)
			case "bigrams":
				v = vocabexperiment.FitMinDF(trainTexts, true, candidate.MinDF)
			default:
				return fmt.Errorf("unknown feature mode %q", candidate.Mode)
			}
			if len(v.Vocabulary) == 0 {
				return fmt.Errorf("empty vocabulary for candidate %s", candidate.Name)
			}
			X := make([][]float64, len(trainTexts))
			for i, text := range trainTexts {
				X[i] = v.Transform(text)
			}
			lr := model.TrainWeighted(X, trainY, model.BalancedWeights(trainY), epochs, learningRate, candidate.L2)
			for _, i := range testIdx {
				rows[i].Scores[candidate.Name] = lr.Predict(v.Transform(groups[i].Texts[0]))
			}
			log.Printf("fold=%d candidate=%s train=%d test=%d vocabulary=%d", fold, candidate.Name, len(trainY), len(testIdx), len(v.Vocabulary))
		}
	}
	if err := os.Mkdir(*out, 0700); err != nil {
		return err
	}
	results := map[string]metrics{}
	for _, candidate := range candidates {
		scores := make([]float64, len(rows))
		for i, row := range rows {
			scores[i] = row.Scores[candidate.Name]
		}
		results[candidate.Name] = summarize(scores, labelsAll)
	}
	labelHash, err := hashFile("data/reviewed_labels.csv")
	if err != nil {
		return err
	}
	manifestHash, err := hashFile(filepath.Join(campaignDir, "manifest.json"))
	if err != nil {
		return err
	}
	protocolOut := map[string]any{
		"purpose":      "grouped five-fold out-of-fold comparison; no validation, dev, or final confirmation scores used",
		"training_ids": protocol.TrainIDs, "reviewed_labels_sha256": labelHash, "frozen_manifest_sha256": manifestHash,
		"exact_conflict_groups_excluded": exactConflictCount, "near_conflict_groups_excluded": nearConflictCount,
		"near_duplicate_rule": "overlap.Similar: normalized exact or >=0.80 five-word shingle Jaccard with >=20 tokens; connected components; one representative per consistent group",
		"folds":               folds, "seed": 20261013,
		"candidates": candidates,
		"features":   "all vocabularies and IDF values are fitted independently within each fold, using only that fold's training data",
		"model":      map[string]any{"l2_grid": l2Grid, "epochs": epochs, "learning_rate": learningRate, "class_weighting": "balanced", "fixed_threshold": decisionCut},
	}
	if err := writeJSON(filepath.Join(*out, "protocol.json"), protocolOut); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(*out, "oof-predictions.json"), rows); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(*out, "metrics.json"), results); err != nil {
		return err
	}
	for _, candidate := range candidates {
		m := results[candidate.Name]
		log.Printf("OOF candidate=%s n=%d P=%.3f R=%.3f F1=%.3f AP=%.3f Brier=%.4f TP/FP/TN/FN=%d/%d/%d/%d", candidate.Name, m.N, m.Precision, m.Recall, m.F1, m.AveragePrecision, m.Brier, m.TP, m.FP, m.TN, m.FN)
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
