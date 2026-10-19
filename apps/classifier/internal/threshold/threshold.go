// apps/classifier/internal/threshold/threshold.go
//
// Decision-threshold selection, mirroring internal/evalmetrics.PickThreshold
// but sweeping the distinct observed score values themselves rather than a
// fixed 0.01 grid. Fine resolution matters when regularization concentrates
// scores near .5 — a fixed grid can land multiple attainable boundaries on
// the same candidate and miss the actual best cut point.
package threshold

import (
	"fmt"
	"sort"
)

func confusionAt(scores, labels []float64, t float64) (precision, recall, f1 float64) {
	var tp, fp, fn int
	for i, score := range scores {
		predicted := score >= t
		actual := labels[i] == 1
		switch {
		case predicted && actual:
			tp++
		case predicted && !actual:
			fp++
		case !predicted && actual:
			fn++
		}
	}
	if tp+fp > 0 {
		precision = float64(tp) / float64(tp+fp)
	}
	if tp+fn > 0 {
		recall = float64(tp) / float64(tp+fn)
	}
	if precision+recall > 0 {
		f1 = 2 * precision * recall / (precision + recall)
	}
	return
}

// Pick sweeps every distinct observed score as a candidate threshold and
// returns the one maximizing recall among those reaching targetPrecision
// (subject to a minRecall floor), falling back to the F1-maximizing
// threshold if none reaches the target.
func Pick(scores, labels []float64, targetPrecision, minRecall float64) (float64, error) {
	if len(scores) == 0 || len(scores) != len(labels) {
		return 0, fmt.Errorf("threshold.Pick: scores and labels must be non-empty and the same length (got %d, %d)", len(scores), len(labels))
	}

	candidates := append([]float64(nil), scores...)
	sort.Float64s(candidates)
	unique := candidates[:0]
	var last float64
	for i, c := range candidates {
		if i == 0 || c != last {
			unique = append(unique, c)
			last = c
		}
	}

	bestPrecisionIdx := -1
	var bestPrecisionRecall float64
	bestF1Idx := -1
	var bestF1 float64
	for i, t := range unique {
		precision, recall, f1 := confusionAt(scores, labels, t)
		if precision >= targetPrecision && (bestPrecisionIdx == -1 || recall > bestPrecisionRecall) {
			bestPrecisionIdx = i
			bestPrecisionRecall = recall
		}
		if bestF1Idx == -1 || f1 > bestF1 {
			bestF1Idx = i
			bestF1 = f1
		}
	}

	if bestPrecisionIdx != -1 && bestPrecisionRecall >= minRecall {
		return unique[bestPrecisionIdx], nil
	}
	return unique[bestF1Idx], nil
}
