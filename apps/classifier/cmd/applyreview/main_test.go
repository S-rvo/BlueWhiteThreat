package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/review"
)

func TestMergeProvenanceAndLegacyDecisions(t *testing.T) {
	dir := t.TempDir()
	store := filepath.Join(dir, "labels.csv")
	batch := filepath.Join(dir, "decisions.jsonl")
	if err := os.WriteFile(store, []byte("id,label\nold,1\ncorrected,0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	decisions := `{"id":"corrected","label":1,"source":"llm_reviewed"}
{"id":"new-human","label":0}
{"id":"missing-label"}
{"id":"invalid-source","label":1,"source":"robot"}
{"id":"invalid-label","label":2}
{"id":"","label":0}
`
	if err := os.WriteFile(batch, []byte(decisions), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DECISIONS_PATH", batch)
	t.Setenv("REVIEWED_LABELS_PATH", store)
	want := map[string]review.Label{
		"old":       {Label: 1, Source: review.SourceHuman},
		"corrected": {Label: 1, Source: review.SourceLLM},
		"new-human": {Label: 0, Source: review.SourceHuman},
	}
	for i := 0; i < 2; i++ {
		main()
		got, err := review.LoadReviewed(store)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %+v, want %+v", got, want)
		}
	}
}
