// apps/classifier/internal/evalmetrics/metrics.go
//
// Shared confusion-matrix / threshold-selection helpers, factored out of
// cmd/train's main.go so comparison tools (cmd/compareheuristics,
// cmd/evalheuristics) don't each carry their own copy.
package evalmetrics

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
)

// ReadLabeledCSV reads a 3-column (id,text,label) CSV with a header row.
func ReadLabeledCSV(path string) (ids []string, texts []string, labels []float64, err error) {
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
		if len(row) != 3 {
			continue
		}
		label, perr := strconv.ParseFloat(row[2], 64)
		if perr != nil {
			continue
		}
		ids = append(ids, row[0])
		texts = append(texts, row[1])
		labels = append(labels, label)
	}
	return ids, texts, labels, nil
}

func ConfusionCounts(scores, labels []float64, threshold float64) (tp, fp, tn, fn int) {
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

func ConfusionAt(scores, labels []float64, threshold float64) (accuracy, precision, recall, f1 float64) {
	tp, fp, tn, fn := ConfusionCounts(scores, labels, threshold)
	total := len(scores)
	accuracy = SafeDiv(float64(tp+tn), float64(total))
	precision = SafeDiv(float64(tp), float64(tp+fp))
	recall = SafeDiv(float64(tp), float64(tp+fn))
	f1 = SafeDiv(2*precision*recall, precision+recall)
	return
}

// PickThreshold sweeps candidate thresholds on (scores, labels) and picks the
// one maximizing recall among those reaching targetPrecision (subject to a
// minRecall floor), falling back to the F1-maximizing threshold if none
// reaches the target. Returns the threshold plus a human-readable reason.
func PickThreshold(scores, labels []float64, targetPrecision, minRecall float64) (threshold float64, reason string) {
	type candidate struct {
		threshold                       float64
		accuracy, precision, recall, f1 float64
	}
	var candidates []candidate
	for t := 0.01; t < 1.0; t += 0.01 {
		accuracy, precision, recall, f1 := ConfusionAt(scores, labels, t)
		candidates = append(candidates, candidate{t, accuracy, precision, recall, f1})
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
		bestByPrecisionFloor = -1
	}

	if bestByPrecisionFloor != -1 {
		c := candidates[bestByPrecisionFloor]
		return c.threshold, fmt.Sprintf("reaches target precision %.2f (precision: %.3f, recall: %.3f, f1: %.3f)",
			targetPrecision, c.precision, c.recall, c.f1)
	}

	c := candidates[bestByF1]
	return c.threshold, fmt.Sprintf("no threshold reached target precision %.2f; using F1-maximizing threshold (precision: %.3f, recall: %.3f, f1: %.3f)",
		targetPrecision, c.precision, c.recall, c.f1)
}

func SafeDiv(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}
