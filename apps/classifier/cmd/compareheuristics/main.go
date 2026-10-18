// apps/classifier/cmd/compareheuristics/main.go
//
// Controlled comparison of the OLD (pre-cherry-pick) and NEW (Codex's
// longest-non-overlapping-phrase) keyword-counting heuristics, isolating the
// heuristic's own effect on model quality from the split-instability bug
// fixed in internal/split/split.go. Both variants are trained and evaluated
// on exactly the same frozen texts, frozen groups, and frozen train/val/test
// partition (internal/frozen) — the only thing that differs between the two
// runs is which labels this tool fills in for documents that have no human
// review.
//
// See cmd/evalheuristics for the follow-up comparison that also includes a
// human-annotation-only reference model and a heuristic-disagreement audit.
//
// Frozen partition construction:
//  1. Exact-duplicate groups are formed by normalized text ONLY (ignoring
//     label), so group membership can't drift between the two variants —
//     texts haven't changed (verified separately: no re-scrape has run with
//     Codex's new HTML-extraction logic since this corpus was fetched).
//  2. Those groups are assigned to train/val/test once, via the (now
//     stable) split.GroupAndSplit, using the current production label
//     (new heuristic + human overrides) purely as the stratification key —
//     this fixes *which* document IDs land in which partition, independent
//     of which heuristic variant is later used to label them.
//  3. The test partition is restricted to documents with a human-reviewed
//     ground-truth label (data/reviewed_labels.csv) — this is the
//     "heuristic-independent annotated test" requested: its labels come
//     from neither heuristic variant, so it fairly judges both.
//
// Per variant (old, new):
//   - Train labels = that variant's heuristic for non-reviewed documents,
//     human-reviewed label otherwise (identical rule cmd/bootstrap uses).
//   - Any frozen group whose members disagree under this variant's own
//     labels is dropped from that variant's train/val (a per-variant
//     conflict, analogous to dedup.GroupByNormalizedText's Conflict).
//   - Decision threshold and Platt calibration are fit ONLY on this
//     variant's own validation split/labels, never on the shared test set.
//   - Final metrics are reported on the frozen, human-ground-truth-only
//     test set, which both variants share byte-for-byte.
//
// Both resulting models (model_old.json / model_new.json) and their metrics
// are provisional: this tool deliberately skips near-duplicate (SimHash)
// merging, since that step is itself label-dependent and would reintroduce
// cross-variant partition drift — exact-text groups are the atomic split
// unit here instead.
package main

import (
	"log"
	"os"
	"time"

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
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env found or unreadable, continuing with existing environment.")
	}

	trainingDataPath := getEnvOrDefault("TRAINING_DATA", "training_data.csv")
	keywordsPath := getEnvOrDefault("SEED_KEYWORDS_PATH", "data/seed_keywords.json")
	reviewedPath := getEnvOrDefault("REVIEWED_LABELS_PATH", "data/reviewed_labels.csv")

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

	// Per-variant label maps: human-reviewed override wins for both
	// variants (identical rule to cmd/bootstrap); otherwise each variant's
	// own heuristic fills in the label.
	oldLabels := make(map[string]float64, len(ids))
	newLabels := make(map[string]float64, len(ids))
	flips := 0
	for i, id := range ids {
		if human, ok := reviewed[id]; ok {
			oldLabels[id] = float64(human)
			newLabels[id] = float64(human)
			continue
		}
		o := float64(heuristiccompare.OldLabelByKeywords(texts[i], keywords))
		n := float64(bootstrap.LabelByKeywords(texts[i], keywords))
		oldLabels[id] = o
		newLabels[id] = n
		if o != n {
			flips++
		}
	}
	log.Printf("Heuristic label delta on non-reviewed documents: %d flips out of %d", flips, len(ids)-len(reviewed))

	// Step 1+2: frozen exact-text groups and ONE frozen partition, using the
	// current production label (new heuristic + human overrides) purely to
	// stratify.
	textGroups := frozen.GroupByTextOnly(ids, texts)
	log.Printf("Frozen %d exact-text groups from %d documents", len(textGroups), len(ids))

	refLabels := make(map[string]float64, len(ids))
	for i, id := range ids {
		refLabels[id] = currentLabels[i]
	}
	trainIDs, valIDs, testIDs := frozen.Partition(textGroups, refLabels, splitSeed, trainRatio, valRatio)
	log.Printf("Frozen partition: %d train ids, %d val ids, %d test ids", len(trainIDs), len(valIDs), len(testIDs))

	// Step 3: heuristic-independent test set — restrict the frozen test
	// partition to documents with a human-reviewed ground-truth label.
	var testTexts []string
	var testGroundTruth []float64
	for id := range testIDs {
		if human, ok := reviewed[id]; ok {
			testTexts = append(testTexts, idText[id])
			testGroundTruth = append(testGroundTruth, float64(human))
		}
	}
	log.Printf("Heuristic-independent test set: %d/%d frozen test ids have human ground truth (rest dropped, not heuristic-independent)",
		len(testGroundTruth), len(testIDs))
	if len(testGroundTruth) == 0 {
		log.Fatal("no human-reviewed ground truth available in the frozen test partition — cannot run a heuristic-independent comparison")
	}

	runVariant("old", oldLabels, textGroups, trainIDs, valIDs, idText, testTexts, testGroundTruth, "model_old.json")
	runVariant("new", newLabels, textGroups, trainIDs, valIDs, idText, testTexts, testGroundTruth, "model_new.json")

	log.Println("=== PROVISIONAL RESULTS — see model_old.json / model_new.json and the per-variant test reports above ===")
	log.Println("These are provisional per explicit instruction: not a final decision between heuristic variants.")
}

// runVariant trains and evaluates one heuristic variant on the shared
// frozen partition, dropping any frozen group whose members disagree under
// this variant's own labels, and reports final metrics on the shared,
// heuristic-independent test set.
func runVariant(name string, variantLabels map[string]float64, textGroups []frozen.TextGroup, trainIDs, valIDs map[string]bool,
	idText map[string]string, testTexts []string, testGroundTruth []float64, modelPath string) {

	log.Printf("=== Variant %q ===", name)

	var trainTexts, valTexts []string
	var trainLabels, valLabels []float64
	droppedTrain, droppedVal := 0, 0

	for _, g := range textGroups {
		consistent := true
		label := variantLabels[g.IDs[0]]
		for _, id := range g.IDs[1:] {
			if variantLabels[id] != label {
				consistent = false
				break
			}
		}

		anyTrain, anyVal := false, false
		for _, id := range g.IDs {
			if trainIDs[id] {
				anyTrain = true
			}
			if valIDs[id] {
				anyVal = true
			}
		}

		if !consistent {
			if anyTrain {
				droppedTrain += len(g.IDs)
			}
			if anyVal {
				droppedVal += len(g.IDs)
			}
			continue
		}

		for _, id := range g.IDs {
			text := idText[id]
			switch {
			case trainIDs[id]:
				trainTexts = append(trainTexts, text)
				trainLabels = append(trainLabels, label)
			case valIDs[id]:
				valTexts = append(valTexts, text)
				valLabels = append(valLabels, label)
			}
		}
	}

	if droppedTrain > 0 || droppedVal > 0 {
		log.Printf("variant %q: dropped %d train / %d val documents from groups inconsistent under this variant's own labels",
			name, droppedTrain, droppedVal)
	}
	log.Printf("variant %q: training on %d examples, validating on %d, testing on %d (shared frozen test)",
		name, len(trainTexts), len(valTexts), len(testTexts))

	vectorizer := tfidf.Fit(trainTexts, trainLabels)

	X := make([][]float64, len(trainTexts))
	for i, t := range trainTexts {
		X[i] = vectorizer.Transform(t)
	}
	weights := model.BalancedWeights(trainLabels)
	lr := model.TrainWeighted(X, trainLabels, weights, epochs, learningRate, l2)

	valScores := make([]float64, len(valTexts))
	for i, t := range valTexts {
		valScores[i] = lr.Predict(vectorizer.Transform(t))
	}
	threshold, reason := evalmetrics.PickThreshold(valScores, valLabels, targetPrecision, minRecall)
	log.Printf("variant %q: chosen threshold %.2f on validation — %s", name, threshold, reason)
	accuracy, precision, recall, f1 := evalmetrics.ConfusionAt(valScores, valLabels, threshold)
	log.Printf("variant %q: validation metrics @ threshold %.2f — accuracy: %.3f, precision: %.3f, recall: %.3f, f1: %.3f (n=%d)",
		name, threshold, accuracy, precision, recall, f1, len(valScores))

	calibration := model.FitPlattScaling(valScores, valLabels, calibrationEpochs, calibrationLearningRate)
	log.Printf("variant %q: fitted Platt scaling A=%.4f, B=%.4f", name, calibration.A, calibration.B)

	testScores := make([]float64, len(testTexts))
	for i, t := range testTexts {
		testScores[i] = lr.Predict(vectorizer.Transform(t))
	}
	tAccuracy, tPrecision, tRecall, tF1 := evalmetrics.ConfusionAt(testScores, testGroundTruth, threshold)
	tp, fp, tn, fn := evalmetrics.ConfusionCounts(testScores, testGroundTruth, threshold)
	log.Printf("variant %q: TEST (heuristic-independent, ground-truth) metrics @ threshold %.2f — accuracy: %.3f, precision: %.3f, recall: %.3f, f1: %.3f (n=%d)",
		name, threshold, tAccuracy, tPrecision, tRecall, tF1, len(testScores))
	log.Printf("variant %q: TEST confusion matrix — TP: %d, FP: %d, TN: %d, FN: %d", name, tp, fp, tn, fn)

	m := &model.Model{
		Vocabulary:  vectorizer.Vocabulary,
		IDF:         vectorizer.IDF,
		Weights:     lr.Weights,
		Bias:        lr.Bias,
		Threshold:   threshold,
		Calibration: calibration,
		Version:     "provisional-" + name + "-" + time.Now().Format("2006-01-02T15-04-05"),
	}
	if err := model.Save(modelPath, m); err != nil {
		log.Fatalf("variant %q: failed to save model: %v", name, err)
	}
	log.Printf("variant %q: saved provisional model to %s (version %s)", name, modelPath, m.Version)
}

func getEnvOrDefault(key, defaultValue string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultValue
}
