package split

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/dedup"
)

func makeGroups(t *testing.T, ids, texts []string, labels []float64) []dedup.Group {
	t.Helper()
	groups, conflicts := dedup.GroupByNormalizedText(ids, texts, labels)
	if len(conflicts) != 0 {
		t.Fatalf("unexpected conflicts in test fixture: %v", conflicts)
	}
	return groups
}

func TestGroupAndSplit_GroupStaysInOneSplit(t *testing.T) {
	var ids, texts []string
	var labels []float64

	dupID := "dup"
	dupText := "This is a duplicated CTI relevant article about CVE-2024-1234."
	for i := 0; i < 5; i++ {
		ids = append(ids, fmt.Sprintf("%s-%d", dupID, i))
		texts = append(texts, dupText)
		labels = append(labels, 1)
	}
	for i := 0; i < 10; i++ {
		ids = append(ids, fmt.Sprintf("pos-%d", i))
		texts = append(texts, fmt.Sprintf("unique positive doc %d", i))
		labels = append(labels, 1)
	}
	for i := 0; i < 10; i++ {
		ids = append(ids, fmt.Sprintf("neg-%d", i))
		texts = append(texts, fmt.Sprintf("unique negative doc %d", i))
		labels = append(labels, 0)
	}

	groups := makeGroups(t, ids, texts, labels)
	train, val, test := GroupAndSplit(groups, 42, 0.6, 0.2)

	contains := func(s Set) bool {
		for _, text := range s.Texts {
			if text == dupText {
				return true
			}
		}
		return false
	}

	present := 0
	count := 0
	for _, s := range []Set{train, val, test} {
		if contains(s) {
			present++
			for _, text := range s.Texts {
				if text == dupText {
					count++
				}
			}
		}
	}
	if present != 1 {
		t.Fatalf("expected the duplicate group to land in exactly one split, found in %d", present)
	}
	if count != 5 {
		t.Fatalf("expected all 5 duplicate members together, found %d", count)
	}
}

func TestGroupAndSplit_Deterministic(t *testing.T) {
	var ids, texts []string
	var labels []float64
	for i := 0; i < 20; i++ {
		ids = append(ids, fmt.Sprintf("pos-%d", i))
		texts = append(texts, fmt.Sprintf("positive %d", i))
		labels = append(labels, 1)
		ids = append(ids, fmt.Sprintf("neg-%d", i))
		texts = append(texts, fmt.Sprintf("negative %d", i))
		labels = append(labels, 0)
	}
	groups := makeGroups(t, ids, texts, labels)

	train1, val1, test1 := GroupAndSplit(groups, 7, 0.6, 0.2)
	train2, val2, test2 := GroupAndSplit(groups, 7, 0.6, 0.2)

	if !reflect.DeepEqual(train1, train2) || !reflect.DeepEqual(val1, val2) || !reflect.DeepEqual(test1, test2) {
		t.Fatal("expected identical split across runs with the same seed")
	}
}

func TestGroupAndSplit_StratifiesByLabel(t *testing.T) {
	var ids, texts []string
	var labels []float64
	for i := 0; i < 80; i++ {
		ids = append(ids, fmt.Sprintf("neg-%d", i))
		texts = append(texts, fmt.Sprintf("negative %d", i))
		labels = append(labels, 0)
	}
	for i := 0; i < 20; i++ {
		ids = append(ids, fmt.Sprintf("pos-%d", i))
		texts = append(texts, fmt.Sprintf("positive %d", i))
		labels = append(labels, 1)
	}
	groups := makeGroups(t, ids, texts, labels)

	train, val, test := GroupAndSplit(groups, 1, 0.7, 0.15)

	countPos := func(s Set) int {
		n := 0
		for _, l := range s.Labels {
			if l == 1 {
				n++
			}
		}
		return n
	}
	if countPos(train) == 0 || countPos(val) == 0 || countPos(test) == 0 {
		t.Fatalf("expected positives in all three splits, got train=%d val=%d test=%d",
			countPos(train), countPos(val), countPos(test))
	}
}

func TestGroupAndSplit_PreservesIDs(t *testing.T) {
	ids := []string{"only-id"}
	texts := []string{"a single document"}
	labels := []float64{1}
	groups := makeGroups(t, ids, texts, labels)

	train, val, test := GroupAndSplit(groups, 1, 0.7, 0.15)
	all := append(append(train.IDs, val.IDs...), test.IDs...)
	if len(all) != 1 || all[0] != "only-id" {
		t.Fatalf("expected the single document's id to be preserved, got %v", all)
	}
}
