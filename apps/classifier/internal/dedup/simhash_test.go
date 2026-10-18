// apps/classifier/internal/dedup/simhash_test.go
package dedup

import "testing"

func TestSimHash_IdenticalTextsMatch(t *testing.T) {
	a := simHash("exploit for sale, contact via telegram for details")
	b := simHash("exploit for sale, contact via telegram for details")
	if a != b {
		t.Fatalf("expected identical simhashes for identical text, got %x vs %x", a, b)
	}
}

func TestSimHash_NearDuplicatesAreClose(t *testing.T) {
	a := simHash("new ransomware strain targets healthcare providers across the region")
	b := simHash("new ransomware strain targets healthcare providers across the area")
	dist := hammingDistance(a, b)
	if dist > 10 {
		t.Fatalf("expected near-duplicate texts to have a small hamming distance, got %d", dist)
	}
}

func TestSimHash_UnrelatedTextsAreFar(t *testing.T) {
	a := simHash("new ransomware strain targets healthcare providers across the region")
	b := simHash("quarterly earnings report shows modest growth in retail sector")
	dist := hammingDistance(a, b)
	if dist < 20 {
		t.Fatalf("expected unrelated texts to have a large hamming distance, got %d", dist)
	}
}

func TestMergeNearDuplicates_MergesCloseGroupsWithSameLabel(t *testing.T) {
	groups := []Group{
		{Key: "k1", IDs: []string{"a1"}, Texts: []string{"new ransomware strain targets healthcare providers across the region"}, Label: 1},
		{Key: "k2", IDs: []string{"a2"}, Texts: []string{"new ransomware strain targets healthcare providers across the area"}, Label: 1},
		{Key: "k3", IDs: []string{"b1"}, Texts: []string{"quarterly earnings report shows modest growth in retail sector"}, Label: 0},
	}

	merged, conflicts := MergeNearDuplicates(groups, 10)

	if len(conflicts) != 0 {
		t.Fatalf("expected no conflicts, got %v", conflicts)
	}
	if len(merged) != 2 {
		t.Fatalf("expected 2 groups after merging the near-duplicate pair, got %d: %v", len(merged), merged)
	}

	var mergedPair *Group
	for i := range merged {
		if len(merged[i].IDs) == 2 {
			mergedPair = &merged[i]
		}
	}
	if mergedPair == nil {
		t.Fatalf("expected one merged group with 2 members, got %v", merged)
	}
	if mergedPair.IDs[0] != "a1" || mergedPair.IDs[1] != "a2" {
		t.Fatalf("expected merged ids [a1 a2], got %v", mergedPair.IDs)
	}
}

func TestMergeNearDuplicates_ConflictsOnDisagreeingLabels(t *testing.T) {
	groups := []Group{
		{Key: "k1", IDs: []string{"a1"}, Texts: []string{"new ransomware strain targets healthcare providers across the region"}, Label: 1},
		{Key: "k2", IDs: []string{"a2"}, Texts: []string{"new ransomware strain targets healthcare providers across the area"}, Label: 0},
	}

	merged, conflicts := MergeNearDuplicates(groups, 10)

	if len(merged) != 0 {
		t.Fatalf("expected no clean merged groups, got %v", merged)
	}
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict for the label-disagreeing near-duplicate pair, got %d: %v", len(conflicts), conflicts)
	}
}

func TestMergeNearDuplicates_LeavesDistinctGroupsAlone(t *testing.T) {
	groups := []Group{
		{Key: "k1", IDs: []string{"a1"}, Texts: []string{"new ransomware strain targets healthcare providers"}, Label: 1},
		{Key: "k2", IDs: []string{"b1"}, Texts: []string{"quarterly earnings report shows modest growth"}, Label: 0},
	}

	merged, conflicts := MergeNearDuplicates(groups, 10)

	if len(conflicts) != 0 {
		t.Fatalf("expected no conflicts, got %v", conflicts)
	}
	if len(merged) != 2 {
		t.Fatalf("expected the 2 distinct groups to remain separate, got %d: %v", len(merged), merged)
	}
}
