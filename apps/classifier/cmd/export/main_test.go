// apps/classifier/cmd/export/main_test.go
package main

import "testing"

func TestPrioritizeNeedsReview_PutsFlaggedCandidatesFirst(t *testing.T) {
	candidates := []ReviewCandidate{
		{ID: "a", NeedsReview: false},
		{ID: "b", NeedsReview: true},
		{ID: "c", NeedsReview: false},
		{ID: "d", NeedsReview: true},
	}

	prioritizeNeedsReview(candidates)

	if !candidates[0].NeedsReview || !candidates[1].NeedsReview {
		t.Fatalf("expected the two needs_review candidates first, got order %v", ids(candidates))
	}
	if candidates[0].ID != "b" || candidates[1].ID != "d" {
		t.Fatalf("expected needs_review candidates to keep their relative order [b d], got %v", ids(candidates))
	}
	if candidates[2].ID != "a" || candidates[3].ID != "c" {
		t.Fatalf("expected non-needs_review candidates to keep their relative order [a c] after the flagged ones, got %v", ids(candidates))
	}
}

func TestPrioritizeNeedsReview_NoFlaggedCandidatesLeavesOrderUnchanged(t *testing.T) {
	candidates := []ReviewCandidate{
		{ID: "a"},
		{ID: "b"},
		{ID: "c"},
	}

	prioritizeNeedsReview(candidates)

	if ids(candidates) != "a,b,c" {
		t.Fatalf("expected order unchanged when nothing is flagged, got %v", ids(candidates))
	}
}

func ids(candidates []ReviewCandidate) string {
	out := ""
	for i, c := range candidates {
		if i > 0 {
			out += ","
		}
		out += c.ID
	}
	return out
}
