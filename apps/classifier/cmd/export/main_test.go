// apps/classifier/cmd/export/main_test.go
package main

import (
	"encoding/json"
	"testing"
)

func TestReviewCandidatePreservesOptionalSourceURL(t *testing.T) {
	for _, url := range []string{"", "https://example.org/report?source=archive"} {
		input := ReviewCandidate{ID: "a", URL: url, Content: "Report text"}
		raw, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		var output ReviewCandidate
		if err := json.Unmarshal(raw, &output); err != nil {
			t.Fatal(err)
		}
		if output.URL != url || output.ID != input.ID || output.Content != input.Content {
			t.Fatalf("source changed: %+v", output)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		if _, present := fields["url"]; present != (url != "") {
			t.Fatal("legacy documents must omit absent URLs")
		}
	}
}

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
