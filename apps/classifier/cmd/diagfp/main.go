// apps/classifier/cmd/diagfp/main.go
// Diagnostic: reproduces the train/eval split from cmd/train and prints
// false positives / false negatives at a given threshold, so we can see
// what's actually confusing the model. Not part of the production pipeline.
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sort"
	"strconv"

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
)

func main() {
	threshold := flag.Float64("threshold", 0.99, "decision threshold to inspect")
	limit := flag.Int("limit", 30, "max examples to print per category")
	flag.Parse()

	_ = godotenv.Load()
	inputPath := getEnvOrDefault("TRAINING_DATA", "training_data.csv")

	ids, texts, labels, err := readLabeledCSV(inputPath)
	if err != nil {
		log.Fatalf("read: %v", err)
	}

	trainIdx, evalIdx := split(labels, trainSplitRatio)

	trainTexts := make([]string, len(trainIdx))
	trainLabels := make([]float64, len(trainIdx))
	for i, idx := range trainIdx {
		trainTexts[i] = texts[idx]
		trainLabels[i] = labels[idx]
	}

	vectorizer := tfidf.Fit(trainTexts, trainLabels)
	X := make([][]float64, len(trainTexts))
	for i, t := range trainTexts {
		X[i] = vectorizer.Transform(t)
	}
	weights := model.BalancedWeights(trainLabels)
	lr := model.TrainWeighted(X, trainLabels, weights, epochs, learningRate, l2)

	type result struct {
		id    string
		text  string
		label float64
		score float64
	}
	var falsePos, falseNeg []result
	var tp, fp, tn, fn int
	for _, idx := range evalIdx {
		score := lr.Predict(vectorizer.Transform(texts[idx]))
		predicted := 0.0
		if score >= *threshold {
			predicted = 1
		}
		r := result{ids[idx], texts[idx], labels[idx], score}
		switch {
		case predicted == 1 && labels[idx] == 1:
			tp++
		case predicted == 1 && labels[idx] == 0:
			fp++
			falsePos = append(falsePos, r)
		case predicted == 0 && labels[idx] == 0:
			tn++
		case predicted == 0 && labels[idx] == 1:
			fn++
			falseNeg = append(falseNeg, r)
		}
	}

	fmt.Printf("threshold=%.2f  tp=%d fp=%d tn=%d fn=%d\n\n", *threshold, tp, fp, tn, fn)

	sort.Slice(falsePos, func(i, j int) bool { return falsePos[i].score > falsePos[j].score })
	sort.Slice(falseNeg, func(i, j int) bool { return falseNeg[i].score > falseNeg[j].score })

	fmt.Printf("=== FALSE POSITIVES (predicted relevant, actually not) — %d total ===\n", len(falsePos))
	for i, r := range falsePos {
		if i >= *limit {
			break
		}
		snippet := r.text
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		fmt.Printf("score=%.3f id=%s text=%q\n", r.score, r.id, snippet)
	}

	fmt.Printf("\n=== FALSE NEGATIVES (predicted not relevant, actually relevant) — %d total ===\n", len(falseNeg))
	for i, r := range falseNeg {
		if i >= *limit {
			break
		}
		snippet := r.text
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		fmt.Printf("score=%.3f id=%s text=%q\n", r.score, r.id, snippet)
	}

	// Full-corpus disagreement scan: among ALL examples (train+eval), how many
	// disagreements are against a human-reviewed label vs. a never-reviewed
	// heuristic label? This tells us whether "errors" are real model mistakes
	// or just unreviewed bootstrap noise.
	reviewed := loadReviewedIDs(getEnvOrDefault("REVIEWED_LABELS", "data/reviewed_labels.csv"))
	var fpReviewed, fpHeuristic, fnReviewed, fnHeuristic int
	for i := range texts {
		score := lr.Predict(vectorizer.Transform(texts[i]))
		predicted := 0.0
		if score >= *threshold {
			predicted = 1
		}
		isReviewed := reviewed[ids[i]]
		switch {
		case predicted == 1 && labels[i] == 0 && isReviewed:
			fpReviewed++
		case predicted == 1 && labels[i] == 0 && !isReviewed:
			fpHeuristic++
		case predicted == 0 && labels[i] == 1 && isReviewed:
			fnReviewed++
		case predicted == 0 && labels[i] == 1 && !isReviewed:
			fnHeuristic++
		}
	}
	fmt.Printf("\n=== FULL CORPUS disagreement breakdown (n=%d) ===\n", len(texts))
	fmt.Printf("false positives: %d against human-reviewed label, %d against unreviewed heuristic label\n", fpReviewed, fpHeuristic)
	fmt.Printf("false negatives: %d against human-reviewed label, %d against unreviewed heuristic label\n", fnReviewed, fnHeuristic)

	var tpR, fpR, fnR, tnR int
	for i := range texts {
		if !reviewed[ids[i]] {
			continue
		}
		score := lr.Predict(vectorizer.Transform(texts[i]))
		predicted := 0.0
		if score >= *threshold {
			predicted = 1
		}
		switch {
		case predicted == 1 && labels[i] == 1:
			tpR++
		case predicted == 1 && labels[i] == 0:
			fpR++
		case predicted == 0 && labels[i] == 0:
			tnR++
		case predicted == 0 && labels[i] == 1:
			fnR++
		}
	}
	precR := float64(tpR) / float64(tpR+fpR)
	recR := float64(tpR) / float64(tpR+fnR)
	fmt.Printf("\n=== Metrics restricted to human-reviewed examples only (n=%d) ===\n", tpR+fpR+tnR+fnR)
	fmt.Printf("tp=%d fp=%d tn=%d fn=%d  precision=%.3f recall=%.3f\n", tpR, fpR, tnR, fnR, precR, recR)
}

func loadReviewedIDs(path string) map[string]bool {
	out := map[string]bool{}
	file, err := os.Open(path)
	if err != nil {
		return out
	}
	defer file.Close()
	reader := csv.NewReader(file)
	rows, err := reader.ReadAll()
	if err != nil {
		return out
	}
	for _, row := range rows[1:] {
		if len(row) >= 1 {
			out[row[0]] = true
		}
	}
	return out
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

	for _, row := range rows[1:] {
		if len(row) != 2 && len(row) != 3 {
			continue
		}
		id := ""
		if len(row) == 3 {
			id = row[0]
		}
		text := row[len(row)-2]
		label, err := strconv.ParseFloat(row[len(row)-1], 64)
		if err != nil {
			continue
		}
		ids = append(ids, id)
		texts = append(texts, text)
		labels = append(labels, label)
	}
	return ids, texts, labels, nil
}

func split(labels []float64, trainRatio float64) (trainIdx, evalIdx []int) {
	rng := rand.New(rand.NewSource(splitSeed))

	classes := map[float64][]int{}
	for i, l := range labels {
		classes[l] = append(classes[l], i)
	}

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
				trainIdx = append(trainIdx, idx)
			} else {
				evalIdx = append(evalIdx, idx)
			}
		}
	}
	return
}

func getEnvOrDefault(key, defaultValue string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultValue
}
