package confirmation

import (
	"encoding/csv"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/model"
)

func csvFile(t *testing.T, path string, rows [][]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := csv.NewWriter(f)
	if err = w.WriteAll(rows); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
}
func setup(t *testing.T) (dir, root string) {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	temp := t.TempDir()
	dir = filepath.Join(temp, "campaign")
	m := &model.Model{Vocabulary: map[string]int{"ransomware": 0}, IDF: []float64{1}, Weights: []float64{4}, Bias: -1, Threshold: 0.5, Version: "provisional-test"}
	if err = model.Save(filepath.Join(temp, "candidate.json"), m); err != nil {
		t.Fatal(err)
	}
	corpus := filepath.Join(temp, "corpus.csv")
	csvFile(t, corpus, [][]string{{"id", "text", "label"}, {"used", "Fresh apples pears orchard harvest farming seasons", "0"}})
	if _, err = Freeze(dir, filepath.Join(temp, "candidate.json"), corpus, root); err != nil {
		t.Fatal(err)
	}
	return
}
func ready(t *testing.T, dir, root string) string {
	t.Helper()
	pool := filepath.Join(t.TempDir(), "pool.csv")
	csvFile(t, pool, [][]string{{"id", "text"}, {"new-a", "ransomware attack encrypts hospital records"}, {"new-b", "basketball championship final score stadium match"}})
	s, err := Prepare(dir, pool, root, 2, 42)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Documents) != 2 {
		t.Fatalf("unexpected exclusion: %+v", s)
	}
	labels := filepath.Join(dir, "annotations.csv")
	csvFile(t, labels, [][]string{{"id", "text", "label"}, {"new-a", "ransomware attack encrypts hospital records", "1"}, {"new-b", "basketball championship final score stadium match", "0"}})
	return labels
}
func TestStrictAnnotations(t *testing.T) {
	for _, rows := range [][][]string{
		{{"id", "text", "label"}, {"a", "text", ""}},
		{{"id", "text", "label"}, {"a", "text", "NaN"}},
		{{"id", "text", "label"}, {"a", "text", "2"}},
		{{"id", "text", "label"}, {"a", "text", "1"}, {"a", "other", "0"}},
		{{"id", "text", "label"}, {"a", " ", "0"}},
		{{"text", "id", "label"}, {"text", "a", "0"}},
	} {
		path := filepath.Join(t.TempDir(), "bad.csv")
		csvFile(t, path, rows)
		if _, err := ReadDocuments(path, true); err == nil {
			t.Fatalf("accepted invalid CSV: %v", rows)
		}
	}
}
func TestLeakageGuards(t *testing.T) {
	ref := []Document{{ID: "used", Text: "alpha beta gamma delta epsilon zeta eta theta"}}
	cases := []Document{
		{ID: "used", Text: "different content"},
		{ID: "new", Text: " ALPHA\u00a0beta gamma delta epsilon zeta eta theta "},
		{ID: "new", Text: "theta eta zeta epsilon delta gamma beta alpha"},
	}
	for _, d := range cases {
		if err := checkBatch([]Document{d}, ref); err == nil {
			t.Fatalf("accepted leakage: %s", d.ID)
		}
	}
	if err := checkBatch([]Document{ref[0], {ID: "other", Text: ref[0].Text}}, nil); err == nil {
		t.Fatal("accepted within-batch duplicate")
	}
}
func TestFreezeDetectsTamperingAndDoesNotOverwrite(t *testing.T) {
	for _, name := range []string{"model.json", "reference.csv"} {
		t.Run(name, func(t *testing.T) {
			dir, root := setup(t)
			if _, err := Freeze(dir, filepath.Join(dir, "model.json"), filepath.Join(dir, "reference.csv"), root); err == nil {
				t.Fatal("overwrote campaign")
			}
			if err := os.WriteFile(filepath.Join(dir, name), []byte("changed"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(dir, root); err == nil {
				t.Fatal("accepted changed artifact")
			}
		})
	}
	dir, root := setup(t)
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := Load(dir, root)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), c.Manifest.RuntimeSHA256, "old-runtime", 1))
	if err = os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = Load(dir, root); err == nil {
		t.Fatal("accepted runtime mismatch")
	}
}
func TestConfirmationConsumesCampaignAndPersistsEvidence(t *testing.T) {
	dir, root := setup(t)
	labels := ready(t, dir, root)
	result, err := Confirm(dir, labels, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.TP != 1 || result.TN != 1 || result.FP != 0 || result.FN != 0 || result.TargetSupported {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.Precision95 == nil || result.Precision95.Low >= 0.99 {
		t.Fatal("tiny sample claimed 99% confidence")
	}
	for _, file := range []string{"attempt.json", "result.json"} {
		if _, err = os.Stat(filepath.Join(dir, file)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = Confirm(dir, labels, root); err == nil {
		t.Fatal("allowed second confirmation")
	}
	csvFile(t, labels, [][]string{{"id", "text", "label"}, {"new-b", "basketball championship final score stadium match", "0"}, {"new-a", "ransomware attack encrypts hospital records", "0"}})
	if _, err = Confirm(dir, labels, root); err == nil {
		t.Fatal("allowed reordered/relabelled confirmation")
	}
}
func TestConcurrentConfirmationScoresOnlyOnce(t *testing.T) {
	dir, root := setup(t)
	labels := ready(t, dir, root)
	var wg sync.WaitGroup
	out := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := Confirm(dir, labels, root); out <- err }()
	}
	wg.Wait()
	close(out)
	success := 0
	for err := range out {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("want one successful confirmation, got %d", success)
	}
}
func TestPartialBatchRejectedBeforeConsumption(t *testing.T) {
	dir, root := setup(t)
	labels := ready(t, dir, root)
	csvFile(t, labels, [][]string{{"id", "text", "label"}, {"new-a", "ransomware attack encrypts hospital records", "1"}})
	if _, err := Confirm(dir, labels, root); err == nil {
		t.Fatal("accepted partial sample")
	}
	if _, err := os.Stat(filepath.Join(dir, "attempt.json")); !os.IsNotExist(err) {
		t.Fatal("invalid batch consumed campaign")
	}
}
func TestBatchDigestIndependentOfOrder(t *testing.T) {
	a := Document{ID: "a", Text: "one", Label: 1}
	b := Document{ID: "b", Text: "two", Label: 0}
	if batchDigest([]Document{a, b}) != batchDigest([]Document{b, a}) {
		t.Fatal("order-dependent digest")
	}
	changed := b
	changed.Label = 1
	if batchDigest([]Document{a, b}) == batchDigest([]Document{a, changed}) {
		t.Fatal("labels missing from digest")
	}
}
func TestWilson95(t *testing.T) {
	if Wilson95(0, 0) != nil {
		t.Fatal("undefined precision must have no interval")
	}
	ci := Wilson95(50, 100)
	if math.Abs(ci.Low-0.40383153) > 1e-6 || math.Abs(ci.High-0.59616847) > 1e-6 {
		t.Fatalf("unexpected interval: %+v", ci)
	}
}

func TestConsumedBatchCannotConfirmAnotherCandidate(t *testing.T) {
	dir, root := setup(t)
	labels := ready(t, dir, root)
	other := filepath.Join(filepath.Dir(dir), "other-campaign")
	if _, err := Freeze(other, filepath.Join(dir, "model.json"), filepath.Join(dir, "reference.csv"), root); err != nil {
		t.Fatal(err)
	}
	otherLabels := ready(t, other, root)
	if _, err := Confirm(dir, labels, root); err != nil {
		t.Fatal(err)
	}
	if _, err := Confirm(other, otherLabels, root); err == nil {
		t.Fatal("reused consumed documents in another campaign")
	}
}

func TestHistoricalBlankTextKeepsItsID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reference.csv")
	csvFile(t, path, [][]string{{"id", "text", "label"}, {"blank-used", "  ", "0"}})
	docs, err := readDocuments(path, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || docs[0].ID != "blank-used" {
		t.Fatal("lost historical id")
	}
	if err := checkBatch([]Document{{ID: "blank-used", Text: "new real content"}}, docs); err == nil {
		t.Fatal("historical id became eligible")
	}
	if _, err = ReadDocuments(path, true); err == nil {
		t.Fatal("accepted blank confirmation text")
	}
}
