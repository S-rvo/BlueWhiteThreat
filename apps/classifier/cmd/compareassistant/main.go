// compareassistant evaluates assistant-generated pseudo-labels as an optional
// training-only augmentation. It never reads the frozen test partition.
package main

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sort"
	"strconv"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/confirmation"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/evalmetrics"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/frozen"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/model"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/overlap"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/review"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/tfidf"
)

const (
	threshold    = 0.5
	epochs       = 4000
	learningRate = 2.0
	l2           = 0.5
)

type metrics struct {
	N         int     `json:"n"`
	TP        int     `json:"tp"`
	FP        int     `json:"fp"`
	TN        int     `json:"tn"`
	FN        int     `json:"fn"`
	Accuracy  float64 `json:"accuracy"`
	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`
	F1        float64 `json:"f1"`
}

type report struct {
	Purpose                       string     `json:"purpose"`
	TrainingCorpus                string     `json:"training_corpus"`
	AssistantLabels               string     `json:"assistant_labels"`
	FrozenCorpusSHA256            string     `json:"frozen_corpus_sha256"`
	PartitionProtocolSHA256       string     `json:"partition_protocol_sha256"`
	Model                         string     `json:"model"`
	Threshold                     float64    `json:"fixed_threshold"`
	ValidationN                   int        `json:"human_validation_n"`
	HumanTrainingN                int        `json:"human_training_n"`
	HumanTrainingExcludedNearVal  int        `json:"human_training_excluded_near_validation"`
	PseudoEligibleN               int        `json:"pseudo_eligible_training_groups"`
	PseudoUsedN                   int        `json:"pseudo_used_training_groups"`
	PseudoExcludedReviewedN       int        `json:"pseudo_excluded_human_reviewed_ids"`
	PseudoExcludedHoldoutN        int        `json:"pseudo_excluded_val_or_test_ids"`
	PseudoExcludedNearValidationN int        `json:"pseudo_excluded_near_validation"`
	PseudoExcludedNearHumanTrainN int        `json:"pseudo_excluded_near_human_training"`
	PseudoHumanTrainOverlapN      int        `json:"pseudo_human_train_overlap_n"`
	PseudoHumanTrainAgreement     int        `json:"pseudo_human_train_agreement"`
	PseudoHumanValidationOverlapN int        `json:"pseudo_human_validation_overlap_n"`
	PseudoHumanValidationAgree    int        `json:"pseudo_human_validation_agreement"`
	Human                         metrics    `json:"human_only"`
	Augmented                     metrics    `json:"human_plus_assistant_pseudo"`
	F1Delta                       float64    `json:"f1_delta_augmented_minus_human"`
	F1DeltaCI95                   [2]float64 `json:"paired_bootstrap_f1_delta_ci95"`
	Interpretation                string     `json:"interpretation"`
}

type evaluation struct {
	Metrics metrics
	Scores  []float64
}

type pseudoLabel struct {
	Label      int
	Confidence float64
}

func main() {
	corpusPath := envOr("TRAINING_DATA", "training_data.csv")
	reviewedPath := envOr("REVIEWED_LABELS_PATH", "data/reviewed_labels.csv")
	annotationsPath := envOr("ASSISTANT_ANNOTATIONS", "data/human-review-batch-max-20261013/assistant_annotations.csv")
	campaign, err := confirmation.Load("data/confirmation-campaign", ".")
	if err != nil {
		log.Fatal(err)
	}
	corpusBytes, err := os.ReadFile(corpusPath)
	if err != nil {
		log.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(corpusBytes)) != campaign.Manifest.CorpusSHA256 {
		log.Fatal("training corpus differs from the frozen confirmation campaign")
	}
	protocolBytes, err := os.ReadFile("data/score-investigation-20261013/protocol.json")
	if err != nil {
		log.Fatal(err)
	}
	var protocol struct {
		ModelSHA256    string   `json:"model_sha256"`
		ReviewedSHA256 string   `json:"reviewed_labels_sha256"`
		TrainIDs       []string `json:"train_ids"`
		ValIDs         []string `json:"val_ids"`
	}
	if err = json.Unmarshal(protocolBytes, &protocol); err != nil {
		log.Fatal(err)
	}
	if protocol.ModelSHA256 != campaign.Manifest.ModelSHA256 {
		log.Fatal("partition protocol references a different frozen model")
	}
	reviewedBytes, err := os.ReadFile(reviewedPath)
	if err != nil {
		log.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(reviewedBytes)) != protocol.ReviewedSHA256 {
		log.Fatal("reviewed labels changed since the partition protocol was frozen")
	}
	reviewedLabels, err := review.LoadReviewed(reviewedPath)
	if err != nil {
		log.Fatal(err)
	}
	reviewed := map[string]int{}
	for id, l := range reviewedLabels {
		reviewed[id] = l.Label
	}
	pseudo, err := readPseudo(annotationsPath)
	if err != nil {
		log.Fatal(err)
	}

	idText, ids, labels := make(map[string]string, len(campaign.Corpus)), make([]string, len(campaign.Corpus)), make(map[string]float64, len(campaign.Corpus))
	texts := make([]string, len(campaign.Corpus))
	for i, doc := range campaign.Corpus {
		idText[doc.ID], ids[i], texts[i], labels[doc.ID] = doc.Text, doc.ID, doc.Text, doc.Label
	}
	groups := frozen.GroupByTextOnly(ids, texts)
	trainIDs, valIDs, testIDs := frozen.Partition(groups, labels, 42, 0.7, 0.15)
	if err := verifyProtocolSets(protocol.TrainIDs, protocol.ValIDs, trainIDs, valIDs, reviewed); err != nil {
		log.Fatal(err)
	}

	valIDsSorted := make([]string, 0, len(valIDs))
	for id := range valIDs {
		if _, ok := reviewed[id]; ok {
			valIDsSorted = append(valIDsSorted, id)
		}
	}
	sort.Strings(valIDsSorted)
	var validationTexts []string
	var validationLabels []float64
	for _, id := range valIDsSorted {
		validationTexts = append(validationTexts, idText[id])
		validationLabels = append(validationLabels, float64(reviewed[id]))
	}
	if len(validationLabels) == 0 {
		log.Fatal("no human-reviewed examples in frozen validation split")
	}
	valPrints := make([]overlap.Print, len(validationTexts))
	for i, text := range validationTexts {
		valPrints[i] = overlap.Fingerprint(text)
	}

	var rawHumanTexts []string
	var rawHumanLabels []float64
	for _, g := range groups {
		for _, id := range g.IDs {
			if trainIDs[id] {
				if label, ok := reviewed[id]; ok {
					rawHumanTexts = append(rawHumanTexts, idText[id])
					rawHumanLabels = append(rawHumanLabels, float64(label))
				}
			}
		}
	}
	var humanTexts []string
	var humanLabels []float64
	humanExcludedNearVal := 0
	for i, text := range rawHumanTexts {
		fingerprint := overlap.Fingerprint(text)
		nearVal := false
		for _, valPrint := range valPrints {
			if overlap.Similar(fingerprint, valPrint) {
				nearVal = true
				break
			}
		}
		if nearVal {
			humanExcludedNearVal++
			continue
		}
		humanTexts = append(humanTexts, text)
		humanLabels = append(humanLabels, rawHumanLabels[i])
	}
	if len(humanTexts) == 0 {
		log.Fatal("no human-reviewed examples remain after near-validation deduplication")
	}
	humanPrints := make([]overlap.Print, len(humanTexts))
	for i, text := range humanTexts {
		humanPrints[i] = overlap.Fingerprint(text)
	}

	pseudoByGroup := map[string]map[int]bool{}
	groupByID := map[string]frozen.TextGroup{}
	for _, g := range groups {
		for _, id := range g.IDs {
			groupByID[id] = g
		}
	}
	for id, p := range pseudo {
		if _, human := reviewed[id]; human {
			continue
		}
		if !trainIDs[id] {
			continue
		}
		g, ok := groupByID[id]
		if !ok {
			continue
		}
		if pseudoByGroup[g.Key] == nil {
			pseudoByGroup[g.Key] = map[int]bool{}
		}
		pseudoByGroup[g.Key][p.Label] = true
	}

	baseTexts := append([]string(nil), humanTexts...)
	baseLabels := append([]float64(nil), humanLabels...)
	var added int
	var excludedNearVal, excludedNearHuman int
	for _, g := range groups {
		labelsForGroup := pseudoByGroup[g.Key]
		if len(labelsForGroup) == 0 {
			continue
		}
		if len(labelsForGroup) != 1 {
			continue
		}
		// Never supplement a group that contains any human-reviewed ID.
		hasHuman := false
		for _, id := range g.IDs {
			if _, ok := reviewed[id]; ok {
				hasHuman = true
				break
			}
		}
		if hasHuman {
			continue
		}
		text := g.Texts[0]
		fingerprint := overlap.Fingerprint(text)
		nearVal := false
		for _, valPrint := range valPrints {
			if overlap.Similar(fingerprint, valPrint) {
				nearVal = true
				break
			}
		}
		if nearVal {
			excludedNearVal++
			continue
		}
		nearHuman := false
		for _, trainPrint := range humanPrints {
			if overlap.Similar(fingerprint, trainPrint) {
				nearHuman = true
				break
			}
		}
		if nearHuman {
			excludedNearHuman++
			continue
		}
		label := 0
		if labelsForGroup[1] {
			label = 1
		}
		baseTexts = append(baseTexts, text)
		baseLabels = append(baseLabels, float64(label))
		added++
	}

	baselineEval := fitAndScore(humanTexts, humanLabels, validationTexts, validationLabels)
	augmentedEval := fitAndScore(baseTexts, baseLabels, validationTexts, validationLabels)
	trainOverlap, trainAgreement, valOverlap, valAgreement := pseudoAgreement(pseudo, reviewed, trainIDs, valIDs, testIDs)
	result := report{
		Purpose:        "Development comparison only: assistant pseudo-labels are used only for frozen training IDs; human validation is scored at a fixed threshold; frozen test partition is never read or scored.",
		TrainingCorpus: corpusPath, AssistantLabels: annotationsPath,
		FrozenCorpusSHA256: campaign.Manifest.CorpusSHA256, PartitionProtocolSHA256: fmt.Sprintf("%x", sha256.Sum256(protocolBytes)),
		Model:     "TF-IDF mixed + weighted logistic regression; same hyperparameters for both candidates",
		Threshold: threshold, ValidationN: len(validationLabels), HumanTrainingN: len(humanTexts),
		HumanTrainingExcludedNearVal: humanExcludedNearVal,
		PseudoEligibleN:              len(pseudoByGroup), PseudoUsedN: added,
		PseudoExcludedReviewedN: countReviewedPseudo(pseudo, reviewed, trainIDs, valIDs, testIDs), PseudoExcludedHoldoutN: countHoldoutPseudo(pseudo, trainIDs),
		PseudoExcludedNearValidationN: excludedNearVal, PseudoExcludedNearHumanTrainN: excludedNearHuman,
		PseudoHumanTrainOverlapN: trainOverlap, PseudoHumanTrainAgreement: trainAgreement,
		PseudoHumanValidationOverlapN: valOverlap, PseudoHumanValidationAgree: valAgreement,
		Human: baselineEval.Metrics, Augmented: augmentedEval.Metrics,
		F1Delta:     augmentedEval.Metrics.F1 - baselineEval.Metrics.F1,
		F1DeltaCI95: pairedBootstrapF1(baselineEval.Scores, augmentedEval.Scores, validationLabels),
	}
	if result.F1Delta > 0 {
		result.Interpretation = "The pseudo-label candidate improves validation F1 in this development run; it remains provisional and needs fresh independent human confirmation."
	} else {
		result.Interpretation = "No validation F1 improvement; do not promote pseudo-labels or the augmented model."
	}
	if err := os.MkdirAll("data/assistant-pseudo-experiment-20261013", 0755); err != nil {
		log.Fatal(err)
	}
	output := "data/assistant-pseudo-experiment-20261013/metrics.json"
	f, err := os.OpenFile(output, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		log.Fatal(err)
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err = enc.Encode(result); err != nil {
		f.Close()
		log.Fatal(err)
	}
	if err = f.Close(); err != nil {
		log.Fatal(err)
	}
	log.Printf("human-only train=%d val=%d metrics=%+v", len(humanTexts), len(validationLabels), baselineEval.Metrics)
	log.Printf("augmented train=%d (+%d pseudo) metrics=%+v", len(baseTexts), added, augmentedEval.Metrics)
	log.Printf("paired F1 delta=%+.4f bootstrap 95%% CI=[%.4f, %.4f]", result.F1Delta, result.F1DeltaCI95[0], result.F1DeltaCI95[1])
	log.Printf("wrote %s", output)
}

// Keep labels aligned with the CSV's textual representation and reject rows
// that are not explicitly marked as local-model pseudo-labels.
func readPseudo(path string) (map[string]pseudoLabel, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("empty pseudo-label file")
	}
	cols := map[string]int{}
	for i, name := range rows[0] {
		cols[name] = i
	}
	for _, name := range []string{"id", "label", "confidence", "annotator"} {
		if _, ok := cols[name]; !ok {
			return nil, fmt.Errorf("pseudo-label file missing %q", name)
		}
	}
	out := map[string]pseudoLabel{}
	for _, row := range rows[1:] {
		if len(row) != len(rows[0]) || row[cols["annotator"]] != "ollama:llama3.2:3b" {
			continue
		}
		label, err := strconv.Atoi(row[cols["label"]])
		if err != nil || (label != 0 && label != 1) {
			continue
		}
		confidence, _ := strconv.ParseFloat(row[cols["confidence"]], 64)
		id := row[cols["id"]]
		if id == "" {
			continue
		}
		out[id] = pseudoLabel{Label: label, Confidence: confidence}
	}
	return out, nil
}

func fitAndScore(texts []string, labels []float64, valTexts []string, valLabels []float64) evaluation {
	vectorizer := tfidf.Fit(texts, labels)
	x := make([][]float64, len(texts))
	for i, text := range texts {
		x[i] = vectorizer.Transform(text)
	}
	lr := model.TrainWeighted(x, labels, model.BalancedWeights(labels), epochs, learningRate, l2)
	scores := make([]float64, len(valTexts))
	for i, text := range valTexts {
		scores[i] = lr.Predict(vectorizer.Transform(text))
	}
	accuracy, precision, recall, f1 := evalmetrics.ConfusionAt(scores, valLabels, threshold)
	tp, fp, tn, fn := evalmetrics.ConfusionCounts(scores, valLabels, threshold)
	return evaluation{Metrics: metrics{N: len(valLabels), TP: tp, FP: fp, TN: tn, FN: fn, Accuracy: accuracy, Precision: precision, Recall: recall, F1: f1}, Scores: scores}
}

// pairedBootstrapF1 resamples the same validation documents for both models.
// The interval is conditional on these trained models and does not include
// retraining variability.
func pairedBootstrapF1(base, candidate, labels []float64) [2]float64 {
	const samples = 20000
	rng := rand.New(rand.NewSource(20261013))
	deltas := make([]float64, samples)
	for sample := 0; sample < samples; sample++ {
		baseTP, baseFP, baseFN := 0, 0, 0
		candTP, candFP, candFN := 0, 0, 0
		for i := 0; i < len(labels); i++ {
			index := rng.Intn(len(labels))
			truth := int(labels[index])
			b, c := 0, 0
			if base[index] >= threshold {
				b = 1
			}
			if candidate[index] >= threshold {
				c = 1
			}
			if b == 1 && truth == 1 {
				baseTP++
			} else if b == 1 {
				baseFP++
			} else if truth == 1 {
				baseFN++
			}
			if c == 1 && truth == 1 {
				candTP++
			} else if c == 1 {
				candFP++
			} else if truth == 1 {
				candFN++
			}
		}
		basePrecision := evalmetrics.SafeDiv(float64(baseTP), float64(baseTP+baseFP))
		baseRecall := evalmetrics.SafeDiv(float64(baseTP), float64(baseTP+baseFN))
		candPrecision := evalmetrics.SafeDiv(float64(candTP), float64(candTP+candFP))
		candRecall := evalmetrics.SafeDiv(float64(candTP), float64(candTP+candFN))
		baseF1 := evalmetrics.SafeDiv(2*basePrecision*baseRecall, basePrecision+baseRecall)
		candF1 := evalmetrics.SafeDiv(2*candPrecision*candRecall, candPrecision+candRecall)
		deltas[sample] = candF1 - baseF1
	}
	sort.Float64s(deltas)
	return [2]float64{deltas[int(0.025*float64(samples))], deltas[int(0.975*float64(samples))]}
}

func countReviewedPseudo(pseudo map[string]pseudoLabel, reviewed map[string]int, trainIDs, valIDs, testIDs map[string]bool) int {
	n := 0
	for id := range pseudo {
		if !testIDs[id] && (trainIDs[id] || valIDs[id]) {
			if _, ok := reviewed[id]; ok {
				n++
			}
		}
	}
	return n
}
func countHoldoutPseudo(pseudo map[string]pseudoLabel, trainIDs map[string]bool) int {
	n := 0
	for id := range pseudo {
		if !trainIDs[id] {
			n++
		}
	}
	return n
}

func pseudoAgreement(pseudo map[string]pseudoLabel, reviewed map[string]int, trainIDs, valIDs, testIDs map[string]bool) (trainN, trainCorrect, valN, valCorrect int) {
	for id, prediction := range pseudo {
		// Frozen-test labels are deliberately not read for this audit.
		if testIDs[id] {
			continue
		}
		truth, hasTruth := reviewed[id]
		if !hasTruth {
			continue
		}
		if trainIDs[id] {
			trainN++
			if prediction.Label == truth {
				trainCorrect++
			}
		}
		if valIDs[id] {
			valN++
			if prediction.Label == truth {
				valCorrect++
			}
		}
	}
	return
}

func verifyProtocolSets(protocolTrain, protocolVal []string, trainIDs, valIDs map[string]bool, reviewed map[string]int) error {
	check := func(name string, protocolIDs []string, expected map[string]bool) error {
		seen := map[string]bool{}
		for _, id := range protocolIDs {
			if seen[id] || !expected[id] {
				return fmt.Errorf("%s protocol contains duplicate or incorrectly partitioned id %s", name, id)
			}
			if _, ok := reviewed[id]; !ok {
				return fmt.Errorf("%s protocol id %s lacks a human label", name, id)
			}
			seen[id] = true
		}
		for id := range expected {
			if _, isReviewed := reviewed[id]; isReviewed && !seen[id] {
				return fmt.Errorf("%s protocol omits human-reviewed id %s", name, id)
			}
		}
		return nil
	}
	if err := check("train", protocolTrain, trainIDs); err != nil {
		return err
	}
	return check("validation", protocolVal, valIDs)
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
