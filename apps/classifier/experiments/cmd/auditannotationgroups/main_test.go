package main

import "testing"

func TestChangingOnlyOneDuplicateCreatesConflict(t *testing.T) {
	one := 1
	ids := []string{"a", "b"}
	texts := []string{" Same page ", "same page"}
	labels := map[string]int{"a": 0, "b": 0}
	proposals := map[string]proposal{"a": {ID: "a", Label: &one}}
	got := group(ids, texts, labels, proposals)
	if len(got) != 1 || got[0].OriginalConflict || !got[0].OverlayConflict || got[0].Changed != 1 {
		t.Fatalf("expected new conflict, got %+v", got)
	}
	proposals["b"] = proposal{ID: "b", Label: &one}
	got = group(ids, texts, labels, proposals)
	if got[0].OverlayConflict || got[0].Changed != 2 {
		t.Fatalf("expected coherent group correction, got %+v", got)
	}
}

func TestAbstentionRetainsOriginalAndUnrelatedGroupsStayOut(t *testing.T) {
	got := group([]string{"a", "b", "c"}, []string{"same", "SAME", "unrelated"},
		map[string]int{"a": 0, "b": 1, "c": 0}, map[string]proposal{"a": {ID: "a"}})
	if len(got) != 1 || !got[0].OriginalConflict || !got[0].OverlayConflict || got[0].Changed != 0 {
		t.Fatalf("abstention must preserve original conflicts, got %+v", got)
	}
}

func TestNearDuplicatesAreGrouped(t *testing.T) {
	base := "alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu nu xi omicron pi rho sigma tau upsilon phi chi psi omega"
	got := group([]string{"a", "b"}, []string{base, base + " extra"},
		map[string]int{"a": 0, "b": 0}, map[string]proposal{"a": {ID: "a"}})
	if len(got) != 1 || len(got[0].Members) != 2 {
		t.Fatalf("near duplicate missing: %+v", got)
	}
}
