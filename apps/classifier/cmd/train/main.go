// apps/classifier/cmd/train/main.go
package main

import (
	"encoding/csv"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/model"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/tfidf"
	"github.com/joho/godotenv"
)

const (
	trainSplitRatio = 0.8
	epochs          = 4000
	learningRate    = 2.0
	l2              = 0.0001
	splitSeed       = 42
	// targetPrecision is the precision floor we try to hit when picking a
	// decision threshold: among thresholds that reach it on the eval set,
	// we keep the one with the best recall. If no threshold reaches it, we
	// fall back to the threshold that maximizes F1.
	targetPrecision = 0.99
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env found or unreadable, continuing with existing environment.")
	}

	inputPath := getEnvOrDefault("TRAINING_DATA", "training_data.csv")
	outputPath := getEnvOrDefault("MODEL_PATH", "model.json")

	texts, labels, err := readLabeledCSV(inputPath)
	if err != nil {
		log.Fatalf("failed to read training data: %v", err)
	}
	if len(texts) < 10 {
		log.Fatalf("need at least 10 labeled examples, got %d — label more data first", len(texts))
	}

	trainTexts, trainLabels, evalTexts, evalLabels := split(texts, labels, trainSplitRatio)
	log.Printf("Training on %d examples, evaluating on %d (split seed: %d)", len(trainTexts), len(evalTexts), splitSeed)

	vectorizer := tfidf.Fit(trainTexts, trainLabels)

	X := make([][]float64, len(trainTexts))
	for i, t := range trainTexts {
		X[i] = vectorizer.Transform(t)
	}

	// Unweighted loss on an imbalanced dataset (here ~8% positives) biases
	// the model toward the majority class. BalancedWeights makes each class
	// contribute equally to the loss so the model doesn't just learn to
	// always predict "not relevant".
	weights := model.BalancedWeights(trainLabels)
	lr := model.TrainWeighted(X, trainLabels, weights, epochs, learningRate, l2)

	evalScores := make([]float64, len(evalTexts))
	for i, t := range evalTexts {
		evalScores[i] = lr.Predict(vectorizer.Transform(t))
	}
	chosenThreshold := pickThreshold(evalScores, evalLabels)

	m := &model.Model{
		Vocabulary: vectorizer.Vocabulary,
		IDF:        vectorizer.IDF,
		Weights:    lr.Weights,
		Bias:       lr.Bias,
		Threshold:  chosenThreshold,
		Version:    time.Now().Format("2006-01-02T15-04-05"),
	}

	evaluateAt(evalScores, evalLabels, chosenThreshold)

	if err := model.Save(outputPath, m); err != nil {
		log.Fatalf("failed to save model: %v", err)
	}
	log.Printf("Saved model to %s (version %s)", outputPath, m.Version)
}

func readLabeledCSV(path string) (texts []string, labels []float64, err error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("parse csv: %w", err)
	}
	if len(rows) < 2 {
		return nil, nil, fmt.Errorf("expected a header row plus at least one data row")
	}

	skipped := 0
	for _, row := range rows[1:] { // skip header
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
		texts = append(texts, text)
		labels = append(labels, label)
	}
	if skipped > 0 {
		log.Printf("skipped %d malformed rows out of %d", skipped, len(rows)-1)
	}
	return texts, labels, nil
}

// split performs a stratified train/eval split so both classes are
// represented proportionally in the (small) eval set — a plain random split
// on a few dozen examples can leave the eval set with almost no positives,
// making precision/recall/f1 swing wildly between runs.
func split(texts []string, labels []float64, trainRatio float64) (trainTexts []string, trainLabels []float64, evalTexts []string, evalLabels []float64) {
	rng := rand.New(rand.NewSource(splitSeed))

	var classes = map[float64][]int{}
	for i, l := range labels {
		classes[l] = append(classes[l], i)
	}

	// Go map iteration order is randomized per run, which would otherwise make
	// the RNG stream order (and thus the resulting split) different on every
	// run despite the fixed splitSeed. Sort class keys for a stable order.
	keys := make([]float64, 0, len(classes))
	for k := range classes {
		keys = append(keys, k)
	}
	sort.Float64s(keys)

	for _, k := range keys {
		idxs := classes[k]
		perm := rng.Perm(len(idxs))
		cut := int(float64(len(idxs)) * trainRatio)
		for i, p := range perm {
			idx := idxs[p]
			if i < cut {
				trainTexts = append(trainTexts, texts[idx])
				trainLabels = append(trainLabels, labels[idx])
			} else {
				evalTexts = append(evalTexts, texts[idx])
				evalLabels = append(evalLabels, labels[idx])
			}
		}
	}
	return
}

// confusionAt computes accuracy/precision/recall/f1 for scores classified
// as positive when score >= threshold, against the actual labels.
func confusionAt(scores []float64, labels []float64, threshold float64) (accuracy, precision, recall, f1 float64) {
	var truePositive, falsePositive, trueNegative, falseNegative int
	for i, score := range scores {
		predicted := 0
		if score >= threshold {
			predicted = 1
		}
		actual := int(labels[i])

		switch {
		case predicted == 1 && actual == 1:
			truePositive++
		case predicted == 1 && actual == 0:
			falsePositive++
		case predicted == 0 && actual == 0:
			trueNegative++
		case predicted == 0 && actual == 1:
			falseNegative++
		}
	}

	total := len(scores)
	accuracy = float64(truePositive+trueNegative) / float64(total)
	precision = safeDiv(float64(truePositive), float64(truePositive+falsePositive))
	recall = safeDiv(float64(truePositive), float64(truePositive+falseNegative))
	f1 = safeDiv(2*precision*recall, precision+recall)
	return
}

// pickThreshold sweeps candidate thresholds against the eval set and logs
// a precision/recall table, so the chosen cutoff is visible and auditable
// instead of being a blind guess. Among thresholds that reach
// targetPrecision, it keeps the one with the highest recall (the least
// aggressive way to hit the precision floor); if none reach it, it falls
// back to the threshold that maximizes F1.
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

	log.Println("Threshold sweep (eval set):")
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

	if bestByPrecisionFloor != -1 {
		c := candidates[bestByPrecisionFloor]
		log.Printf("Chosen threshold %.2f reaches target precision %.2f (precision: %.3f, recall: %.3f, f1: %.3f)",
			c.threshold, targetPrecision, c.precision, c.recall, c.f1)
		return c.threshold
	}

	c := candidates[bestByF1]
	log.Printf("No threshold on the eval set reaches target precision %.2f; falling back to the F1-maximizing threshold %.2f (precision: %.3f, recall: %.3f, f1: %.3f)",
		targetPrecision, c.threshold, c.precision, c.recall, c.f1)
	return c.threshold
}

func evaluateAt(scores []float64, labels []float64, threshold float64) {
	accuracy, precision, recall, f1 := confusionAt(scores, labels, threshold)
	log.Printf("Eval metrics @ threshold %.2f — accuracy: %.3f, precision: %.3f, recall: %.3f, f1: %.3f (n=%d)",
		threshold, accuracy, precision, recall, f1, len(scores))
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
