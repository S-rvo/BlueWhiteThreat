// comparehumanl2 checks the previously cross-validated L2 candidates on the
// fixed human validation slice. It also diagnoses positive-class weighting.
// It is development only, uses fixed cutoffs, and never writes a model.
package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/evalmetrics"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/frozen"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/model"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/overlap"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/review"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/tfidf"
)

const (
	seed         = 42
	trainRatio   = 0.7
	valRatio     = 0.15
	epochs       = 4000
	learningRate = 2.0
	cutoff       = 0.5
)

func main() {
	dataPath := flag.String("data", "training_data.csv", "labeled corpus CSV")
	reviewedPath := flag.String("reviewed", "data/reviewed_labels.csv", "human labels CSV")
	flag.Parse()
	ids, texts, labels, err := evalmetrics.ReadLabeledCSV(*dataPath)
	if err != nil {
		log.Fatal(err)
	}
	reviewedLabels, err := review.LoadReviewed(*reviewedPath)
	if err != nil {
		log.Fatal(err)
	}
	reviewed := map[string]int{}
	for id, l := range reviewedLabels {
		reviewed[id] = l.Label
	}
	textByID := make(map[string]string, len(ids))
	refLabels := make(map[string]float64, len(ids))
	for i, id := range ids {
		textByID[id], refLabels[id] = texts[i], labels[i]
	}
	groups := frozen.GroupByTextOnly(ids, texts)
	trainIDs, valIDs, _ := frozen.Partition(groups, refLabels, seed, trainRatio, valRatio)
	trainDocs := []string{}
	trainLabels := []float64{}
	valDocs := []string{}
	valLabels := []float64{}
	for _, id := range ids {
		label, ok := reviewed[id]
		if !ok {
			continue
		}
		if trainIDs[id] {
			trainDocs = append(trainDocs, textByID[id])
			trainLabels = append(trainLabels, float64(label))
		}
		if valIDs[id] {
			valDocs = append(valDocs, textByID[id])
			valLabels = append(valLabels, float64(label))
		}
	}
	valPrints := make([]overlap.Print, len(valDocs))
	for i, doc := range valDocs {
		valPrints[i] = overlap.Fingerprint(doc)
	}
	keptDocs := make([]string, 0, len(trainDocs))
	keptLabels := make([]float64, 0, len(trainLabels))
	removedNear := 0
	for i, doc := range trainDocs {
		print := overlap.Fingerprint(doc)
		nearValidation := false
		for _, reference := range valPrints {
			if overlap.Similar(print, reference) {
				nearValidation = true
				break
			}
		}
		if nearValidation {
			removedNear++
			continue
		}
		keptDocs = append(keptDocs, doc)
		keptLabels = append(keptLabels, trainLabels[i])
	}
	trainDocs, trainLabels = keptDocs, keptLabels
	if len(trainDocs) == 0 || len(valDocs) == 0 {
		log.Fatalf("empty human train/validation: train=%d val=%d", len(trainDocs), len(valDocs))
	}
	fmt.Printf("Human-only train=%d validation=%d; removed %d near-overlapping train docs; fixed cutoff=%.2f; validation is development data\n", len(trainDocs), len(valDocs), removedNear, cutoff)
	var bestL2Scores []float64
	for _, l2 := range []float64{0.5, 0.05, 0.005} {
		v := tfidf.Fit(trainDocs, trainLabels)
		X := make([][]float64, len(trainDocs))
		for i, doc := range trainDocs {
			X[i] = v.Transform(doc)
		}
		lr := model.TrainWeighted(X, trainLabels, model.BalancedWeights(trainLabels), epochs, learningRate, l2)
		scores := make([]float64, len(valDocs))
		for i, doc := range valDocs {
			scores[i] = lr.Predict(v.Transform(doc))
		}
		if l2 == 0.005 {
			bestL2Scores = scores
		}
		tp, fp, tn, fn := evalmetrics.ConfusionCounts(scores, valLabels, cutoff)
		accuracy, precision, recall, f1 := evalmetrics.ConfusionAt(scores, valLabels, cutoff)
		fmt.Printf("L2=%.3g P=%.3f R=%.3f F1=%.3f acc=%.3f TP/FP/TN/FN=%d/%d/%d/%d\n", l2, precision, recall, f1, accuracy, tp, fp, tn, fn)
	}
	fmt.Println("L2=0.005 threshold diagnostic (same development validation; no threshold selected):")
	for _, threshold := range []float64{0.50, 0.60, 0.70, 0.80, 0.90, 0.95, 0.99} {
		_, precision, recall, f1 := evalmetrics.ConfusionAt(bestL2Scores, valLabels, threshold)
		tp, fp, tn, fn := evalmetrics.ConfusionCounts(bestL2Scores, valLabels, threshold)
		fmt.Printf("  threshold=%.2f P=%.3f R=%.3f F1=%.3f predicted-positive=%d (TP=%d FP=%d); TN=%d FN=%d\n", threshold, precision, recall, f1, tp+fp, tp, fp, tn, fn)
	}
	fmt.Println("L2=0.005 positive-weight diagnostic (fixed threshold=0.50):")
	baselineWeights := model.BalancedWeights(trainLabels)
	positiveMultipliers := []float64{0.125, 0.25, 0.5, 1.0, 2.0}
	for _, multiplier := range positiveMultipliers {
		weights := append([]float64(nil), baselineWeights...)
		for i, label := range trainLabels {
			if label == 1 {
				weights[i] *= multiplier
			}
		}
		v := tfidf.Fit(trainDocs, trainLabels)
		X := make([][]float64, len(trainDocs))
		for i, doc := range trainDocs {
			X[i] = v.Transform(doc)
		}
		lr := model.TrainWeighted(X, trainLabels, weights, epochs, learningRate, 0.005)
		scores := make([]float64, len(valDocs))
		for i, doc := range valDocs {
			scores[i] = lr.Predict(v.Transform(doc))
		}
		tp, fp, tn, fn := evalmetrics.ConfusionCounts(scores, valLabels, cutoff)
		_, precision, recall, f1 := evalmetrics.ConfusionAt(scores, valLabels, cutoff)
		fmt.Printf("  positive_multiplier=%.3g P=%.3f R=%.3f F1=%.3f TP/FP/TN/FN=%d/%d/%d/%d\n", multiplier, precision, recall, f1, tp, fp, tn, fn)
	}
}
