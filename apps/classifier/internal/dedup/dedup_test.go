package dedup

import "testing"

func TestNormalize_CollapsesCaseAndWhitespace(t *testing.T) {
	a := Normalize("Hello   World\n\n")
	b := Normalize("hello world")
	if a != b {
		t.Fatalf("expected equal normalized forms, got %q vs %q", a, b)
	}
}

func TestGroupByNormalizedText_GroupsDuplicates(t *testing.T) {
	ids := []string{"a1", "a2", "b1"}
	texts := []string{"Same Article  Text", "same article text", "a totally different article"}
	labels := []float64{1, 1, 0}

	groups, conflicts := GroupByNormalizedText(ids, texts, labels)

	if len(conflicts) != 0 {
		t.Fatalf("expected no conflicts, got %v", conflicts)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups (1 duplicate pair + 1 unique), got %d", len(groups))
	}

	var dupGroup *Group
	for i := range groups {
		if len(groups[i].IDs) == 2 {
			dupGroup = &groups[i]
		}
	}
	if dupGroup == nil {
		t.Fatalf("expected one group with 2 members, got %v", groups)
	}
	if dupGroup.IDs[0] != "a1" || dupGroup.IDs[1] != "a2" {
		t.Fatalf("expected duplicate group ids [a1 a2], got %v", dupGroup.IDs)
	}
	if dupGroup.Label != 1 {
		t.Fatalf("expected duplicate group label 1, got %v", dupGroup.Label)
	}
}

func TestGroupByNormalizedText_FlagsConflictingLabels(t *testing.T) {
	ids := []string{"a1", "a2"}
	texts := []string{"Ambiguous Article", "ambiguous article"}
	labels := []float64{1, 0}

	groups, conflicts := GroupByNormalizedText(ids, texts, labels)

	if len(groups) != 0 {
		t.Fatalf("expected no clean groups, got %v", groups)
	}
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(conflicts))
	}
	if conflicts[0].IDs[0] != "a1" || conflicts[0].IDs[1] != "a2" {
		t.Fatalf("expected conflict ids [a1 a2], got %v", conflicts[0].IDs)
	}
}
