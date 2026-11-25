package dedup

import (
	"fmt"
	"testing"
)

func numberedTokens(from, to int) string {
	text := ""
	for i := from; i <= to; i++ {
		if text != "" {
			text += " "
		}
		text += fmt.Sprintf("token%d", i)
	}
	return text
}

func TestMergeContentDuplicatesDoesNotMergeTopicalText(t *testing.T) {
	groups := []Group{
		{Key: "a", IDs: []string{"a"}, Texts: []string{"security vulnerability vendor patch update advisory remote access ransomware incident response affected product version"}, Label: 1},
		{Key: "b", IDs: []string{"b"}, Texts: []string{"security vulnerability vendor patch update advisory database access initial broker malware campaign unrelated prose"}, Label: 1},
	}
	got, conflicts := MergeContentDuplicates(groups)
	if len(got) != 2 || len(conflicts) != 0 {
		t.Fatalf("got %d groups, %d conflicts", len(got), len(conflicts))
	}
}

func TestMergeContentDuplicatesMergesMinorRecrawl(t *testing.T) {
	base := "one two three four five six seven eight nine ten eleven twelve thirteen fourteen fifteen sixteen seventeen eighteen nineteen twenty extra"
	recrawl := "one two three four five six seven eight nine ten eleven twelve thirteen fourteen fifteen sixteen seventeen eighteen nineteen twenty changed"
	groups := []Group{{Key: "a", IDs: []string{"a"}, Texts: []string{base}, Label: 0}, {Key: "b", IDs: []string{"b"}, Texts: []string{recrawl}, Label: 0}}
	got, conflicts := MergeContentDuplicates(groups)
	if len(got) != 1 || len(conflicts) != 0 {
		t.Fatalf("got %d groups, %d conflicts", len(got), len(conflicts))
	}
}

func TestMergeContentDuplicatesClosesSimilarityTransitively(t *testing.T) {
	base := numberedTokens(1, 30)
	a := numberedTokens(101, 104) + " " + base
	c := base + " " + numberedTokens(201, 204)
	groups := []Group{
		{Key: "a", IDs: []string{"a"}, Texts: []string{a}, Label: 0},
		{Key: "c", IDs: []string{"c"}, Texts: []string{c}, Label: 0},
		{Key: "b", IDs: []string{"b"}, Texts: []string{base}, Label: 0},
	}
	got, conflicts, diagnostics := MergeContentDuplicatesWithDiagnostics(groups)
	if len(got) != 1 || len(got[0].IDs) != 3 || len(conflicts) != 0 {
		t.Fatalf("transitive cluster not merged: got=%#v conflicts=%#v", got, conflicts)
	}
	if diagnostics.LargestComponentDocuments != 3 || diagnostics.TransitiveOnlyComponents != 1 || diagnostics.MultiDocumentComponents != 1 {
		t.Fatalf("unexpected diagnostics: %+v", diagnostics)
	}
	if diagnostics.MaximumComponentDiameter <= 0 || diagnostics.LargestComponentMinimumSimilarity >= 0.8 {
		t.Fatalf("expected transitive component diameter diagnostics: %+v", diagnostics)
	}
}
