package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyProvenanceRejectsChangedInputs(t *testing.T) {
	p := experimentProtocol{TrainIDs: []string{"a", "b"}, ReviewedHash: "labels", ManifestHash: "manifest", Folds: 5}
	p.Model.Threshold = 0.5
	ids := map[string]bool{"a": true, "b": true}
	if err := verifyProvenance(p, ids, "labels", "manifest"); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ name, labels, manifest string }{
		{"changed labels", "new-labels", "manifest"},
		{"changed corpus", "labels", "new-manifest"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := verifyProvenance(p, ids, tt.labels, tt.manifest); err == nil {
				t.Fatal("changed input accepted")
			}
		})
	}
	p.TrainIDs = []string{"a", "a"}
	if err := verifyProvenance(p, ids, "labels", "manifest"); err == nil {
		t.Fatal("duplicate training IDs accepted")
	}
}

func TestReadReferenceIgnoresLabels(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reference.csv")
	if err := os.WriteFile(path, []byte("id,text,label\na,training text,invalid-label\n"), 0600); err != nil {
		t.Fatal(err)
	}
	texts, err := readReference(path)
	if err != nil {
		t.Fatal(err)
	}
	if texts["a"] != "training text" {
		t.Fatal("unexpected reference text")
	}
}

func TestSelectDisagreementsUsesOnlyVerifiedTrainingRows(t *testing.T) {
	rows := []oofRow{
		{ID: "positive-miss", Label: 1, Fold: 0, Scores: map[string]float64{"mixed_base": 0.2}},
		{ID: "negative-hit", Label: 0, Fold: 1, Scores: map[string]float64{"mixed_base": 0.8}},
		{ID: "correct", Label: 1, Fold: 2, Scores: map[string]float64{"mixed_base": 0.7}},
	}
	selected, err := selectDisagreements(
		rows,
		map[string]bool{"positive-miss": true, "negative-hit": true, "correct": true},
		map[string]int{"positive-miss": 1, "negative-hit": 0, "correct": 1},
		map[string]string{"positive-miss": "text one", "negative-hit": "text two", "correct": "text three"},
		map[string]bool{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 2 {
		t.Fatalf("selected %d rows, want 2", len(selected))
	}
	for _, row := range selected {
		if row.ID == "correct" {
			t.Fatal("correctly classified row was selected")
		}
		if row.Text == "" {
			t.Fatalf("selected row %s has no text", row.ID)
		}
	}
}

func TestSelectDisagreementsRejectsRowsOutsideTrainingPartition(t *testing.T) {
	rows := make([]oofRow, 502)
	for i := range rows {
		id := "train"
		if i > 0 {
			id = string(rune('a' + i))
		}
		rows[i] = oofRow{ID: id, Label: 0, Fold: i % 5, Scores: map[string]float64{"mixed_base": 0.1}}
	}
	_, err := selectDisagreements(rows, map[string]bool{}, map[string]int{}, map[string]string{}, map[string]bool{})
	if err == nil {
		t.Fatal("expected an out-of-partition row to be rejected")
	}
}

func TestSelectDisagreementsRejectsExcludedRows(t *testing.T) {
	rows := make([]oofRow, 502)
	trainIDs := map[string]bool{}
	labels := map[string]int{}
	texts := map[string]string{}
	for i := range rows {
		id := "id-" + string(rune('a'+i%26)) + string(rune('A'+i/26))
		trainIDs[id] = true
		labels[id] = 0
		texts[id] = "text"
		rows[i] = oofRow{ID: id, Label: 0, Fold: i % 5, Scores: map[string]float64{"mixed_base": 0.1}}
	}
	_, err := selectDisagreements(rows, trainIDs, labels, texts, map[string]bool{rows[10].ID: true})
	if err == nil {
		t.Fatal("expected an excluded training row to be rejected")
	}
}
