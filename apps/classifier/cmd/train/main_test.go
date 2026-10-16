package main

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeCSV(t *testing.T, rows [][]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "labels.csv")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := csv.NewWriter(f)
	if err := w.WriteAll(rows); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadExcludesDuplicatesAndAllConflictingCopies(t *testing.T) {
	path := writeCSV(t, [][]string{{"text", "label"}, {"Malware attack!", "1"}, {"malware attack", "1"}, {"safe page", "0"}, {"SAFE page!", "1"}, {"different content", "0"}, {"!", "0"}})
	texts, labels, err := readLabeledCSV(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(texts, []string{"Malware attack!", "different content"}) || !reflect.DeepEqual(labels, []float64{1, 0}) {
		t.Fatalf("unexpected cleaned data: %v %v", texts, labels)
	}
}

func TestReadRejectsInvalidLabels(t *testing.T) {
	for _, label := range []string{"NaN", "Inf", "-1", "0.5", "2", "unknown"} {
		path := writeCSV(t, [][]string{{"text", "label"}, {"malware attack", label}})
		if _, _, err := readLabeledCSV(path); err == nil {
			t.Fatalf("accepted %q", label)
		}
	}
}

func TestThreeWaySplitIsDeterministicAndDisjoint(t *testing.T) {
	var texts []string
	var labels []float64
	for i := 0; i < 40; i++ {
		texts = append(texts, string(rune('A'+i)))
		labels = append(labels, float64(i%2))
	}
	dev, dy, test, _ := split(texts, labels, .8)
	train, _, validation, _ := split(dev, dy, .75)
	seen := map[string]bool{}
	for _, partition := range [][]string{train, validation, test} {
		for _, doc := range partition {
			if seen[doc] {
				t.Fatal("split leakage")
			}
			seen[doc] = true
		}
	}
	if len(seen) != len(texts) {
		t.Fatal("lost documents")
	}
	again, _, _, _ := split(texts, labels, .8)
	if !reflect.DeepEqual(dev, again) {
		t.Fatal("nondeterministic split")
	}
}

func TestThresholdUsesObservedScores(t *testing.T) {
	scores := []float64{.501, .502, .503, .504}
	labels := []float64{0, 0, 1, 1}
	threshold := pickThreshold(scores, labels)
	_, precision, recall, f1 := confusionAt(scores, labels, threshold)
	if precision != 1 || recall != 1 || f1 != 1 {
		t.Fatalf("missed separating threshold %g", threshold)
	}
}
