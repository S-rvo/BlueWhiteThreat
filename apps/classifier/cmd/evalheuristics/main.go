// apps/classifier/cmd/evalheuristics/main.go
//
// Follow-up to cmd/compareheuristics. That tool compared the OLD and NEW
// keyword-counting heuristics indirectly, as sources of training labels for
// a trained model. This tool compares three candidates directly as
// classifiers, on a human-annotated validation set:
//
//  1. The OLD heuristic rule (heuristiccompare.OldLabelByKeywords), fixed
//     threshold, no tuning.
//  2. The NEW heuristic rule (heuristiccompare.NewLabelByKeywordsWithMin),
//     swept across candidate minMatches thresholds — Codex's current
//     internal/bootstrap.LabelByKeywords hardcodes MinKeywordHits=3 as a
//     package constant, so this reproduction lets us ask whether a
//     different threshold would do better, without touching that file.
//  3. A reference logistic-regression model trained using ONLY documents
//     that have a human-reviewed ground-truth label (no heuristic-derived
//     labels anywhere in its training set).
//
// All parameters (NEW's minMatches, the reference model's decision
// threshold) are chosen exclusively on validation, per instruction. The
// frozen test partition built by cmd/compareheuristics' sibling logic
// (internal/frozen, same seed/ratios) is loaded here only to be excluded —
// it remains reserved for a later, independent confirmation and is never
// scored in this tool. Documents annotated after the current
// data/reviewed_labels.csv snapshot should be reserved the same way going
// forward, rather than spent on tuning.
//
// Additionally audits where the OLD and NEW heuristics disagree (excluding
// the reserved test IDs), to understand what the automatic heuristics get
// wrong, cross-referenced against ground truth where available. Disagreement
// snippets are truncated and drawn only from the already-vetted
// training_data.csv corpus (no new scraping/content is touched by this
// tool) — per the standing safety constraint, if this ever surfaces a new
// CSAM-adjacent indicator not already covered by the existing exclusion
// list, stop and surface it individually rather than writing it to the CSV.
package main

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"sort"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/bootstrap"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/evalmetrics"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/frozen"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/heuristiccompare"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/model"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/review"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/tfidf"
	"github.com/joho/godotenv"
)

const (
	trainRatio              = 0.7
	valRatio                = 0.15
	splitSeed               = 42
	epochs                  = 4000
	learningRate            = 2.0
	l2                      = 0.5
	targetPrecision         = 0.99
	minRecall               = 0.3
	calibrationEpochs       = 2000
	calibrationLearningRate = 0.1

	minMatchesLow  = 1
	minMatchesHigh = 8

	disagreementSnippetLen = 200
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env found or unreadable, continuing with existing environment.")
	}

	trainingDataPath := getEnvOrDefault("TRAINING_DATA", "training_data.csv")
	keywordsPath := getEnvOrDefault("SEED_KEYWORDS_PATH", "data/seed_keywords.json")
	reviewedPath := getEnvOrDefault("REVIEWED_LABELS_PATH", "data/reviewed_labels.csv")
	disagreementsPath := getEnvOrDefault("DISAGREEMENTS_CSV", "heuristic_disagreements.csv")
	humanModelPath := getEnvOrDefault("HUMAN_MODEL_PATH", "model_human.json")

	ids, texts, currentLabels, err := evalmetrics.ReadLabeledCSV(trainingDataPath)
	if err != nil {
		log.Fatalf("failed to read %s: %v", trainingDataPath, err)
	}
	log.Printf("Loaded %d documents from %s (frozen text corpus)", len(ids), trainingDataPath)

	keywords, err := bootstrap.LoadKeywords(keywordsPath)
	if err != nil {
		log.Fatalf("failed to load seed keywords: %v", err)
	}

	reviewedLabels, err := review.LoadReviewed(reviewedPath)
	if err != nil {
		log.Fatalf("failed to load reviewed labels: %v", err)
	}
	reviewed := map[string]int{}
	for id, l := range reviewedLabels {
		reviewed[id] = l.Label
	}
	log.Printf("Loaded %d human-reviewed ground-truth labels from %s", len(reviewed), reviewedPath)

	idText := make(map[string]string, len(ids))
	for i, id := range ids {
		idText[id] = texts[i]
	}

	// Same frozen partition as cmd/compareheuristics (same groups, same
	// seed/ratios, same stratification label), so trainIDs/valIDs/testIDs
	// line up exactly with that tool's reserved test set.
	textGroups := frozen.GroupByTextOnly(ids, texts)
	refLabels := make(map[string]float64, len(ids))
	for i, id := range ids {
		refLabels[id] = currentLabels[i]
	}
	trainIDs, valIDs, testIDs := frozen.Partition(textGroups, refLabels, splitSeed, trainRatio, valRatio)
	log.Printf("Frozen partition: %d train ids, %d val ids, %d test ids (test ids RESERVED, not used below)",
		len(trainIDs), len(valIDs), len(testIDs))

	// Human-annotated validation subset: the "validation annotée
	// humainement" the instruction asks for.
	var valTexts []string
	var valGroundTruth []float64
	for id := range valIDs {
		if human, ok := reviewed[id]; ok {
			valTexts = append(valTexts, idText[id])
			valGroundTruth = append(valGroundTruth, float64(human))
		}
	}
	log.Printf("Human-annotated validation subset: %d/%d frozen val ids have human ground truth", len(valGroundTruth), len(valIDs))
	if len(valGroundTruth) == 0 {
		log.Fatal("no human-reviewed ground truth available in the frozen validation partition — cannot compare candidates")
	}

	// Human-annotated training subset: for the reference model, which must
	// be trained using ONLY human annotations, no heuristic-derived labels.
	var trainTexts []string
	var trainGroundTruth []float64
	for id := range trainIDs {
		if human, ok := reviewed[id]; ok {
			trainTexts = append(trainTexts, idText[id])
			trainGroundTruth = append(trainGroundTruth, float64(human))
		}
	}
	log.Printf("Human-annotated training subset: %d/%d frozen train ids have human ground truth", len(trainGroundTruth), len(trainIDs))

	// --- Candidate 1: OLD heuristic rule, direct, no tuning ---
	oldScores := make([]float64, len(valTexts))
	for i, t := range valTexts {
		oldScores[i] = float64(heuristiccompare.OldLabelByKeywords(t, keywords))
	}
	oldAcc, oldPrec, oldRec, oldF1 := evalmetrics.ConfusionAt(oldScores, valGroundTruth, 0.5)
	oldTP, oldFP, oldTN, oldFN := evalmetrics.ConfusionCounts(oldScores, valGroundTruth, 0.5)
	log.Printf("[OLD rule] validation (n=%d) — accuracy: %.3f, precision: %.3f, recall: %.3f, f1: %.3f, TP:%d FP:%d TN:%d FN:%d",
		len(valTexts), oldAcc, oldPrec, oldRec, oldF1, oldTP, oldFP, oldTN, oldFN)

	// --- Candidate 2: NEW heuristic rule, swept over minMatches ---
	type sweepResult struct {
		minMatches                      int
		accuracy, precision, recall, f1 float64
		tp, fp, tn, fn                  int
	}
	var sweep []sweepResult
	bestIdx := -1
	for m := minMatchesLow; m <= minMatchesHigh; m++ {
		scores := make([]float64, len(valTexts))
		for i, t := range valTexts {
			scores[i] = float64(heuristiccompare.NewLabelByKeywordsWithMin(t, keywords, m))
		}
		acc, prec, rec, f1 := evalmetrics.ConfusionAt(scores, valGroundTruth, 0.5)
		tp, fp, tn, fn := evalmetrics.ConfusionCounts(scores, valGroundTruth, 0.5)
		sweep = append(sweep, sweepResult{m, acc, prec, rec, f1, tp, fp, tn, fn})
		log.Printf("[NEW rule sweep] minMatches=%d — accuracy: %.3f, precision: %.3f, recall: %.3f, f1: %.3f, TP:%d FP:%d TN:%d FN:%d",
			m, acc, prec, rec, f1, tp, fp, tn, fn)

		// Prefer the highest minMatches reaching the target precision with
		// recall above the floor (mirrors evalmetrics.PickThreshold); fall
		// back to the F1-maximizing minMatches otherwise.
		if prec >= targetPrecision && rec >= minRecall {
			if bestIdx == -1 || sweep[bestIdx].recall < rec {
				bestIdx = len(sweep) - 1
			}
		}
	}
	if bestIdx == -1 {
		for i, s := range sweep {
			if bestIdx == -1 || s.f1 > sweep[bestIdx].f1 {
				bestIdx = i
			}
		}
	}
	best := sweep[bestIdx]
	log.Printf("[NEW rule] chosen minMatches=%d on validation — accuracy: %.3f, precision: %.3f, recall: %.3f, f1: %.3f",
		best.minMatches, best.accuracy, best.precision, best.recall, best.f1)

	// --- Candidate 3: reference model trained ONLY on human annotations ---
	vectorizer := tfidf.Fit(trainTexts, trainGroundTruth)
	X := make([][]float64, len(trainTexts))
	for i, t := range trainTexts {
		X[i] = vectorizer.Transform(t)
	}
	weights := model.BalancedWeights(trainGroundTruth)
	lr := model.TrainWeighted(X, trainGroundTruth, weights, epochs, learningRate, l2)

	refScores := make([]float64, len(valTexts))
	for i, t := range valTexts {
		refScores[i] = lr.Predict(vectorizer.Transform(t))
	}
	refThreshold, refReason := evalmetrics.PickThreshold(refScores, valGroundTruth, targetPrecision, minRecall)
	log.Printf("[human-only reference model] chosen threshold %.2f on validation — %s", refThreshold, refReason)
	refAcc, refPrec, refRec, refF1 := evalmetrics.ConfusionAt(refScores, valGroundTruth, refThreshold)
	refTP, refFP, refTN, refFN := evalmetrics.ConfusionCounts(refScores, valGroundTruth, refThreshold)
	log.Printf("[human-only reference model] validation (n=%d) @ threshold %.2f — accuracy: %.3f, precision: %.3f, recall: %.3f, f1: %.3f, TP:%d FP:%d TN:%d FN:%d",
		len(valTexts), refThreshold, refAcc, refPrec, refRec, refF1, refTP, refFP, refTN, refFN)

	calibration := model.FitPlattScaling(refScores, valGroundTruth, calibrationEpochs, calibrationLearningRate)
	refModel := &model.Model{
		Vocabulary:  vectorizer.Vocabulary,
		IDF:         vectorizer.IDF,
		Weights:     lr.Weights,
		Bias:        lr.Bias,
		Threshold:   refThreshold,
		Calibration: calibration,
		Version:     "provisional-human-only",
	}
	if err := model.Save(humanModelPath, refModel); err != nil {
		log.Fatalf("failed to save human-only reference model: %v", err)
	}
	log.Printf("saved provisional human-only reference model to %s (trained on %d human-annotated documents)", humanModelPath, len(trainTexts))

	// --- Three-way comparison, on the shared human-annotated validation set ---
	log.Println("=== THREE-WAY COMPARISON (human-annotated validation set, n=" + fmt.Sprint(len(valTexts)) + ") ===")
	log.Printf("  OLD rule (fixed):            accuracy %.3f  precision %.3f  recall %.3f  f1 %.3f", oldAcc, oldPrec, oldRec, oldF1)
	log.Printf("  NEW rule (minMatches=%d):     accuracy %.3f  precision %.3f  recall %.3f  f1 %.3f", best.minMatches, best.accuracy, best.precision, best.recall, best.f1)
	log.Printf("  Human-only reference model:  accuracy %.3f  precision %.3f  recall %.3f  f1 %.3f", refAcc, refPrec, refRec, refF1)
	log.Println("These are validation-only metrics: the frozen test partition remains reserved for a later, independent confirmation.")

	// --- Heuristic disagreement audit ---
	auditDisagreements(ids, texts, idText, keywords, reviewed, testIDs, disagreementsPath)

	log.Printf("Confirmed: %d reserved test ids were never scored by this tool. Any documents annotated after this run's "+
		"%s snapshot (%d rows) should likewise be held out for the final confirmation step, not used for tuning.",
		len(testIDs), reviewedPath, len(reviewed))
}

// auditDisagreements finds documents where the OLD and NEW (current,
// shipping) heuristics disagree, excluding reserved test ids, and writes a
// CSV of representative cases (short snippets only) plus a tally against
// ground truth where available.
func auditDisagreements(ids, texts []string, idText map[string]string, keywords []string, reviewed map[string]int,
	testIDs map[string]bool, outPath string) {

	type disagreement struct {
		id             string
		snippet        string
		oldPred        int
		newPred        int
		groundTruth    int
		hasGroundTruth bool
	}
	var disagreements []disagreement
	oldCorrect, newCorrect, bothWrong, noGroundTruth := 0, 0, 0, 0

	for i, id := range ids {
		if testIDs[id] {
			continue // reserved for final confirmation, never inspected here
		}
		text := texts[i]
		oldPred := heuristiccompare.OldLabelByKeywords(text, keywords)
		newPred := bootstrap.LabelByKeywords(text, keywords)
		if oldPred == newPred {
			continue
		}

		snippet := text
		if len(snippet) > disagreementSnippetLen {
			snippet = snippet[:disagreementSnippetLen]
		}

		d := disagreement{id: id, snippet: snippet, oldPred: oldPred, newPred: newPred}
		if human, ok := reviewed[id]; ok {
			d.groundTruth = human
			d.hasGroundTruth = true
			switch {
			case oldPred == human && newPred != human:
				oldCorrect++
			case newPred == human && oldPred != human:
				newCorrect++
			default:
				bothWrong++
			}
		} else {
			noGroundTruth++
		}
		disagreements = append(disagreements, d)
	}

	sort.Slice(disagreements, func(i, j int) bool { return disagreements[i].id < disagreements[j].id })

	log.Printf("Heuristic disagreement audit (excluding %d reserved test ids): %d disagreements out of %d documents",
		len(testIDs), len(disagreements), len(ids))
	log.Printf("  Of disagreements with human ground truth: OLD correct=%d, NEW correct=%d, both wrong=%d, no ground truth=%d",
		oldCorrect, newCorrect, bothWrong, noGroundTruth)

	file, err := os.Create(outPath)
	if err != nil {
		log.Fatalf("failed to create %s: %v", outPath, err)
	}
	defer file.Close()

	w := csv.NewWriter(file)
	defer w.Flush()
	if err := w.Write([]string{"id", "text_snippet", "old_pred", "new_pred", "ground_truth", "has_ground_truth"}); err != nil {
		log.Fatalf("failed to write header to %s: %v", outPath, err)
	}
	for _, d := range disagreements {
		gt := ""
		if d.hasGroundTruth {
			gt = fmt.Sprint(d.groundTruth)
		}
		if err := w.Write([]string{d.id, d.snippet, fmt.Sprint(d.oldPred), fmt.Sprint(d.newPred), gt, fmt.Sprint(d.hasGroundTruth)}); err != nil {
			log.Fatalf("failed to write row to %s: %v", outPath, err)
		}
	}
	log.Printf("Wrote %d disagreement cases to %s for manual inspection", len(disagreements), outPath)
}

func getEnvOrDefault(key, defaultValue string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultValue
}
