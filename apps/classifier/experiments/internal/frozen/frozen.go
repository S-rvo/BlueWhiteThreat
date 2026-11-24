// apps/classifier/internal/frozen/frozen.go
//
// Builds a train/validation/test partition that is frozen independent of
// which label-filling heuristic (old/new keyword rule, human-only, etc.) a
// caller later applies. Groups are formed by normalized text ONLY, ignoring
// label, so group membership can't drift between label variants; the split
// itself is deterministic given the seed, so repeated runs (and runs across
// tools comparing different variants) land every document in the same
// partition.
package frozen

import (
	"hash/fnv"
	"sort"
	"strconv"
	"strings"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/text"
)

// TextGroup is a set of document IDs that share identical normalized text.
type TextGroup struct {
	Key   string
	IDs   []string
	Texts []string
}

// GroupByTextOnly groups ids whose texts normalize to the same token
// sequence, ignoring label entirely.
func GroupByTextOnly(ids, texts []string) []TextGroup {
	order := make([]string, 0, len(ids))
	byKey := make(map[string]*TextGroup)
	for i, id := range ids {
		key := strings.Join(text.Tokenize(texts[i]), " ")
		g, seen := byKey[key]
		if !seen {
			g = &TextGroup{Key: key}
			byKey[key] = g
			order = append(order, key)
		}
		g.IDs = append(g.IDs, id)
		g.Texts = append(g.Texts, texts[i])
	}

	groups := make([]TextGroup, 0, len(order))
	for _, key := range order {
		groups = append(groups, *byKey[key])
	}
	return groups
}

// Partition assigns every group to train, validation, or test, stratified
// by the group's label (the label of its first member) and split
// deterministically via seed. Every member of a group lands in the same
// set, so near-identical documents never straddle splits. The remainder
// after train+validation becomes the test set.
func Partition(groups []TextGroup, labels map[string]float64, seed int64, trainRatio, valRatio float64) (train, val, test map[string]bool) {
	train = map[string]bool{}
	val = map[string]bool{}
	test = map[string]bool{}

	byLabel := map[float64][]TextGroup{}
	for _, g := range groups {
		label := labels[g.IDs[0]]
		byLabel[label] = append(byLabel[label], g)
	}

	var labelKeys []float64
	for l := range byLabel {
		labelKeys = append(labelKeys, l)
	}
	sort.Float64s(labelKeys)

	assign := func(g TextGroup, dst map[string]bool) {
		for _, id := range g.IDs {
			dst[id] = true
		}
	}

	for _, l := range labelKeys {
		gs := append([]TextGroup(nil), byLabel[l]...)
		sort.Slice(gs, func(i, j int) bool {
			return groupSortHash(gs[i], seed) < groupSortHash(gs[j], seed)
		})
		trainCut := int(float64(len(gs)) * trainRatio)
		valCut := trainCut + int(float64(len(gs))*valRatio)
		for i, g := range gs {
			switch {
			case i < trainCut:
				assign(g, train)
			case i < valCut:
				assign(g, val)
			default:
				assign(g, test)
			}
		}
	}
	return train, val, test
}

// groupSortHash derives a deterministic, stable sort key for a group from
// its member IDs (sorted so member order doesn't matter) and the seed, so
// the train/val/test boundary for a label class only shifts for groups
// right at the cut point when the label's group count changes slightly.
func groupSortHash(g TextGroup, seed int64) uint64 {
	ids := append([]string(nil), g.IDs...)
	sort.Strings(ids)
	h := fnv.New64a()
	h.Write([]byte(strconv.FormatInt(seed, 10)))
	h.Write([]byte("|"))
	h.Write([]byte(strings.Join(ids, ",")))
	return h.Sum64()
}
