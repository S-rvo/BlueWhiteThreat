// apps/classifier/cmd/train/main.go
package main

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/dedup"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/embeddings"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/model"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/split"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/tfidf"
	"github.com/joho/godotenv"
)

const (
	// trainRatio/valRatio/testRatio (the remainder after train+val) split
	// deduplicated groups three ways: train fits the vocabulary/IDF/model,
	// validation picks the decision threshold, and test is reserved for a
	// single final report so threshold-picking never leaks into the number
	// we claim as the model's real-world precision/recall.
	trainRatio   = 0.7
	valRatio     = 0.15
	epochs       = 4000
	learningRate = 2.0
	l2           = 0.0001
	splitSeed    = 42
	// targetPrecision is the precision floor we try to hit when picking a
	// decision threshold: among thresholds that reach it on the validation
	// set, we keep the one with the best recall. If no threshold reaches it,
	// we fall back to the threshold that maximizes F1.
	targetPrecision = 0.99
	// minRecall rejects a targetPrecision-meeting threshold whose recall is
	// degenerate — on a small, imbalanced validation set, the precision
	// floor can be reachable only at an extreme threshold supported by a
	// handful of positives, which is a worse real-world operating point
	// than a lower threshold with better-rounded precision/recall that
	// never quite reaches the floor. When the best floor-meeting candidate
	// falls under minRecall, pickThreshold falls back to the F1-maximizing
	// threshold instead.
	minRecall = 0.3
	// nearDuplicateMaxDistance is the maximum SimHash hamming distance (out
	// of 64 bits) for two exact-dedup groups to be treated as the same
	// underlying content (e.g. a re-crawl with minor copy edits, or a
	// lightly paraphrased repost). Calibrated empirically: a single
	// word-level edit in a short CTI blurb produces a handful of flipped
	// bits, while unrelated documents differ in roughly half the bits.
	nearDuplicateMaxDistance = 10
	// calibrationEpochs/calibrationLearningRate control the gradient
	// descent used to fit Platt scaling (2 scalar parameters, so this
	// converges fast even with many epochs).
	calibrationEpochs       = 2000
	calibrationLearningRate = 0.1
	// embeddingDim/embeddingWindow/embeddingMaxVocab/embeddingSeed control
	// the pure-Go semantic word embeddings (see internal/embeddings): a
	// small PPMI co-occurrence table over the embeddingMaxVocab most
	// frequent words, built from a window of embeddingWindow neighboring
	// tokens, randomly projected down to embeddingDim dimensions and
	// averaged per document. This adds a semantic feature channel next to
	// TF-IDF — e.g. letting "malware" and "ransomware" contribute similar
	// signal — without a pre-trained table, an extra runtime, or any
	// network access, per the classifier design doc's constraint against
	// heavy ML dependencies.
	embeddingDim      = 0
	embeddingWindow   = 4
	embeddingMaxVocab = 2000
	embeddingSeed     = 42
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env found or unreadable, continuing with existing environment.")
	}

	inputPath := getEnvOrDefault("TRAINING_DATA", "training_data.csv")
	outputPath := getEnvOrDefault("MODEL_PATH", "model.json")

	ids, texts, labels, err := readLabeledCSV(inputPath)
	if err != nil {
		log.Fatalf("failed to read training data: %v", err)
	}
	if len(texts) < 10 {
		log.Fatalf("need at least 10 labeled examples, got %d — label more data first", len(texts))
	}

	groups, conflicts := dedup.GroupByNormalizedText(ids, texts, labels)
	if len(conflicts) > 0 {
		log.Printf("WARNING: %d set(s) of identical-text documents have contradictory labels; excluding them from training/eval rather than guessing:", len(conflicts))
		for _, c := range conflicts {
			log.Printf("  conflict — ids: %v, labels: %v", c.IDs, c.Labels)
		}
	}

	beforeNearDedup := len(groups)
	var nearDupConflicts []dedup.Conflict
	groups, nearDupConflicts = dedup.MergeNearDuplicates(groups, nearDuplicateMaxDistance)
	log.Printf("Near-duplicate merge: %d groups before, %d after (paraphrased/re-crawled reposts collapsed)", beforeNearDedup, len(groups))
	if len(nearDupConflicts) > 0 {
		log.Printf("WARNING: %d set(s) of near-duplicate documents have contradictory labels; excluding them from training/eval rather than guessing:", len(nearDupConflicts))
		for _, c := range nearDupConflicts {
			log.Printf("  conflict — ids: %v, labels: %v", c.IDs, c.Labels)
		}
	}

	trainSet, valSet, testSet := split.GroupAndSplit(groups, splitSeed, trainRatio, valRatio)
	log.Printf("Training on %d examples, validating on %d, testing on %d (split seed: %d)",
		len(trainSet.Texts), len(valSet.Texts), len(testSet.Texts), splitSeed)

	vectorizer := tfidf.Fit(trainSet.Texts, trainSet.Labels)

	wordVectors := embeddings.BuildWordVectors(trainSet.Texts, embeddingWindow, embeddingDim, embeddingMaxVocab, embeddingSeed)
	embedder := &embeddings.Embedder{Vectors: wordVectors, Dim: embeddingDim}
	log.Printf("Built semantic embeddings for %d words (dim=%d)", len(wordVectors), embeddingDim)

	featureVector := func(t string) []float64 {
		return append(vectorizer.Transform(t), embedder.Transform(t)...)
	}

	X := make([][]float64, len(trainSet.Texts))
	for i, t := range trainSet.Texts {
		X[i] = featureVector(t)
	}

	// Unweighted loss on an imbalanced dataset (here ~8% positives) biases
	// the model toward the majority class. BalancedWeights makes each class
	// contribute equally to the loss so the model doesn't just learn to
	// always predict "not relevant".
	weights := model.BalancedWeights(trainSet.Labels)
	lr := model.TrainWeighted(X, trainSet.Labels, weights, epochs, learningRate, l2)

	valScores := make([]float64, len(valSet.Texts))
	for i, t := range valSet.Texts {
		valScores[i] = lr.Predict(featureVector(t))
	}
	chosenThreshold := pickThreshold(valScores, valSet.Labels)
	evaluateAt("validation", valScores, valSet.Labels, chosenThreshold)

	// Fit Platt scaling on the same validation set used to pick the
	// threshold: TrainWeighted's balanced-loss objective makes raw scores
	// systematically overconfident relative to the true positive rate, so
	// calibration corrects that without touching the threshold decision
	// itself (which stays based on raw scores).
	calibration := model.FitPlattScaling(valScores, valSet.Labels, calibrationEpochs, calibrationLearningRate)
	log.Printf("Fitted Platt scaling calibration: A=%.4f, B=%.4f", calibration.A, calibration.B)

	testScores := make([]float64, len(testSet.Texts))
	for i, t := range testSet.Texts {
		testScores[i] = lr.Predict(featureVector(t))
	}
	reportTest(testScores, testSet.Labels, chosenThreshold)

	m := &model.Model{
		Vocabulary:  vectorizer.Vocabulary,
		IDF:         vectorizer.IDF,
		Weights:     lr.Weights,
		Bias:        lr.Bias,
		Threshold:   chosenThreshold,
		Calibration: calibration,
		Embedding:   *embedder,
		Version:     time.Now().Format("2006-01-02T15-04-05"),
	}

	if err := model.Save(outputPath, m); err != nil {
		log.Fatalf("failed to save model: %v", err)
	}
	log.Printf("Saved model to %s (version %s)", outputPath, m.Version)
}

func readLabeledCSV(path string) (ids []string, texts []string, labels []float64, err error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("parse csv: %w", err)
	}
	if len(rows) < 2 {
		return nil, nil, nil, fmt.Errorf("expected a header row plus at least one data row")
	}

	skipped := 0
	for i, row := range rows[1:] { // skip header
		// Supports both the legacy 2-column format (text,label) and the
		// current 3-column format (id,text,label) — text and label are
		// always the last two fields.
		if len(row) != 2 && len(row) != 3 {
			skipped++
			continue
		}
		text := row[len(row)-2]
		label, err := strconv.ParseFloat(row[len(row)-1], 64)
		if err != nil {
			skipped++
			continue
		}
		id := fmt.Sprintf("row-%d", i+1)
		if len(row) == 3 {
			id = row[0]
		}
		ids = append(ids, id)
		texts = append(texts, text)
		labels = append(labels, label)
	}
	if skipped > 0 {
		log.Printf("skipped %d malformed rows out of %d", skipped, len(rows)-1)
	}
	return ids, texts, labels, nil
}

// confusionCounts tallies true/false positives/negatives for scores
// classified as positive when score >= threshold, against the actual labels.
func confusionCounts(scores []float64, labels []float64, threshold float64) (tp, fp, tn, fn int) {
	for i, score := range scores {
		predicted := 0
		if score >= threshold {
			predicted = 1
		}
		actual := int(labels[i])

		switch {
		case predicted == 1 && actual == 1:
			tp++
		case predicted == 1 && actual == 0:
			fp++
		case predicted == 0 && actual == 0:
			tn++
		case predicted == 0 && actual == 1:
			fn++
		}
	}
	return
}

// confusionAt computes accuracy/precision/recall/f1 for scores classified
// as positive when score >= threshold, against the actual labels.
func confusionAt(scores []float64, labels []float64, threshold float64) (accuracy, precision, recall, f1 float64) {
	tp, fp, tn, fn := confusionCounts(scores, labels, threshold)
	total := len(scores)
	accuracy = float64(tp+tn) / float64(total)
	precision = safeDiv(float64(tp), float64(tp+fp))
	recall = safeDiv(float64(tp), float64(tp+fn))
	f1 = safeDiv(2*precision*recall, precision+recall)
	return
}

// pickThreshold sweeps candidate thresholds against the validation set and
// logs a precision/recall table, so the chosen cutoff is visible and
// auditable instead of being a blind guess. Among thresholds that reach
// targetPrecision, it keeps the one with the highest recall (the least
// aggressive way to hit the precision floor); if none reach it, it falls
// back to the threshold that maximizes F1. The validation set is used here
// so the test set stays untouched until the final report — picking the
// threshold on the same data used to report final metrics would bias the
// reported precision/recall upward.
func pickThreshold(scores []float64, labels []float64) float64 {
	type candidate struct {
		threshold                       float64
		accuracy, precision, recall, f1 float64
	}

	var candidates []candidate
	for t := 0.01; t < 1.0; t += 0.01 {
		accuracy, precision, recall, f1 := confusionAt(scores, labels, t)
		candidates = append(candidates, candidate{t, accuracy, precision, recall, f1})
	}

	log.Println("Threshold sweep (validation set):")
	for _, t := range []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9} {
		_, p, r, f := confusionAt(scores, labels, t)
		log.Printf("  threshold %.2f — precision: %.3f, recall: %.3f, f1: %.3f", t, p, r, f)
	}

	bestByPrecisionFloor := -1
	bestByF1 := 0
	for i, c := range candidates {
		if c.precision >= targetPrecision {
			if bestByPrecisionFloor == -1 || c.recall > candidates[bestByPrecisionFloor].recall {
				bestByPrecisionFloor = i
			}
		}
		if c.f1 > candidates[bestByF1].f1 {
			bestByF1 = i
		}
	}

	if bestByPrecisionFloor != -1 && candidates[bestByPrecisionFloor].recall < minRecall {
		log.Printf("Threshold %.2f reaches target precision %.2f but its recall (%.3f) is below minRecall %.2f; treating the precision floor as unreachable",
			candidates[bestByPrecisionFloor].threshold, targetPrecision, candidates[bestByPrecisionFloor].recall, minRecall)
		bestByPrecisionFloor = -1
	}

	if bestByPrecisionFloor != -1 {
		c := candidates[bestByPrecisionFloor]
		log.Printf("Chosen threshold %.2f reaches target precision %.2f (precision: %.3f, recall: %.3f, f1: %.3f)",
			c.threshold, targetPrecision, c.precision, c.recall, c.f1)
		return c.threshold
	}

	c := candidates[bestByF1]
	log.Printf("No threshold on the validation set reaches target precision %.2f; falling back to the F1-maximizing threshold %.2f (precision: %.3f, recall: %.3f, f1: %.3f)",
		targetPrecision, c.threshold, c.precision, c.recall, c.f1)
	return c.threshold
}

func evaluateAt(setName string, scores []float64, labels []float64, threshold float64) {
	accuracy, precision, recall, f1 := confusionAt(scores, labels, threshold)
	log.Printf("%s metrics @ threshold %.2f — accuracy: %.3f, precision: %.3f, recall: %.3f, f1: %.3f (n=%d)",
		setName, threshold, accuracy, precision, recall, f1, len(scores))
}

// reportTest logs the final, held-out performance report: metrics, raw
// confusion-matrix counts, and an explicit failure warning if the chosen
// threshold doesn't actually reach targetPrecision on data it never
// influenced (neither training nor threshold selection touched the test
// set).
func reportTest(scores []float64, labels []float64, threshold float64) {
	accuracy, precision, recall, f1 := confusionAt(scores, labels, threshold)
	tp, fp, tn, fn := confusionCounts(scores, labels, threshold)
	log.Printf("Test metrics @ threshold %.2f — accuracy: %.3f, precision: %.3f, recall: %.3f, f1: %.3f (n=%d)",
		threshold, accuracy, precision, recall, f1, len(scores))
	log.Printf("Test confusion matrix — TP: %d, FP: %d, TN: %d, FN: %d", tp, fp, tn, fn)
	if precision < targetPrecision {
		log.Printf("WARNING: target precision %.2f NOT met on the test set (actual: %.3f). The model does not meet the precision bar on held-out data.",
			targetPrecision, precision)
	}
}

func safeDiv(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}

func getEnvOrDefault(key, defaultValue string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultValue
}
