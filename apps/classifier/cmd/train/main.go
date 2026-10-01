// apps/classifier/cmd/train/main.go
package main

import (
	"encoding/csv"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/classify"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/model"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/tfidf"
)

const (
	trainSplitRatio = 0.8
	epochs          = 500
	learningRate    = 0.1
	l2              = 0.001
	threshold       = 0.5
	splitSeed       = 42
)

func main() {
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

	vectorizer := tfidf.Fit(trainTexts)

	X := make([][]float64, len(trainTexts))
	for i, t := range trainTexts {
		X[i] = vectorizer.Transform(t)
	}

	lr := model.Train(X, trainLabels, epochs, learningRate, l2)

	m := &model.Model{
		Vocabulary: vectorizer.Vocabulary,
		IDF:        vectorizer.IDF,
		Weights:    lr.Weights,
		Bias:       lr.Bias,
		Threshold:  threshold,
		Version:    time.Now().Format("2006-01-02T15-04-05"),
	}

	evaluate(m, evalTexts, evalLabels)

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
		if len(row) != 2 {
			skipped++
			continue
		}
		label, err := strconv.ParseFloat(row[1], 64)
		if err != nil {
			skipped++
			continue
		}
		texts = append(texts, row[0])
		labels = append(labels, label)
	}
	if skipped > 0 {
		log.Printf("skipped %d malformed rows out of %d", skipped, len(rows)-1)
	}
	return texts, labels, nil
}

func split(texts []string, labels []float64, trainRatio float64) (trainTexts []string, trainLabels []float64, evalTexts []string, evalLabels []float64) {
	n := len(texts)
	rng := rand.New(rand.NewSource(splitSeed))
	indices := rng.Perm(n)
	cut := int(float64(n) * trainRatio)

	for i, idx := range indices {
		if i < cut {
			trainTexts = append(trainTexts, texts[idx])
			trainLabels = append(trainLabels, labels[idx])
		} else {
			evalTexts = append(evalTexts, texts[idx])
			evalLabels = append(evalLabels, labels[idx])
		}
	}
	return
}

func evaluate(m *model.Model, texts []string, labels []float64) {
	c := classify.New(m)

	var truePositive, falsePositive, trueNegative, falseNegative int
	for i, text := range texts {
		result := c.Classify(text)
		predicted := 0
		if result.Relevant {
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

	total := len(texts)
	accuracy := float64(truePositive+trueNegative) / float64(total)
	precision := safeDiv(float64(truePositive), float64(truePositive+falsePositive))
	recall := safeDiv(float64(truePositive), float64(truePositive+falseNegative))
	f1 := safeDiv(2*precision*recall, precision+recall)

	log.Printf("Eval metrics — accuracy: %.3f, precision: %.3f, recall: %.3f, f1: %.3f (n=%d)",
		accuracy, precision, recall, f1, total)
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
