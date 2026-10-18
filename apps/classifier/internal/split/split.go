// apps/classifier/internal/split/split.go
package split

import (
	"hash/fnv"
	"sort"
	"strconv"
	"strings"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/dedup"
)

// Set is one partition of a train/validation/test split, carrying each
// document's original ID alongside its text and label so partitions stay
// auditable and reproducible.
type Set struct {
	IDs    []string
	Texts  []string
	Labels []float64
}

// GroupAndSplit partitions dedup groups into train/validation/test sets,
// stratified by label and split deterministically via seed. Every member of
// a group is assigned to the same set — duplicates (already merged into
// groups by dedup.GroupByNormalizedText) never straddle splits, which would
// otherwise let the model "recognize" an eval example it was trained on and
// inflate reported metrics. The remainder after train+validation becomes
// the test set.
func GroupAndSplit(groups []dedup.Group, seed int64, trainRatio, valRatio float64) (train, val, test Set) {
	byLabel := map[float64][]dedup.Group{}
	for _, g := range groups {
		byLabel[g.Label] = append(byLabel[g.Label], g)
	}

	var labelKeys []float64
	for l := range byLabel {
		labelKeys = append(labelKeys, l)
	}
	sort.Float64s(labelKeys) // deterministic regardless of map iteration order

	assign := func(g dedup.Group, dst *Set) {
		dst.IDs = append(dst.IDs, g.IDs...)
		dst.Texts = append(dst.Texts, g.Texts...)
		for range g.IDs {
			dst.Labels = append(dst.Labels, g.Label)
		}
	}

	for _, l := range labelKeys {
		gs := append([]dedup.Group(nil), byLabel[l]...)
		sort.Slice(gs, func(i, j int) bool {
			return groupSortHash(gs[i], seed) < groupSortHash(gs[j], seed)
		})
		trainCut := int(float64(len(gs)) * trainRatio)
		valCut := trainCut + int(float64(len(gs))*valRatio)
		for i, g := range gs {
			switch {
			case i < trainCut:
				assign(g, &train)
			case i < valCut:
				assign(g, &val)
			default:
				assign(g, &test)
			}
		}
	}
	return
}

// groupSortHash derives a deterministic, stable sort key for a group from
// its member IDs (sorted so member order doesn't matter) and the seed, so
// that the train/val/test boundary for a label class only shifts for groups
// right at the cut point when the label's group count changes slightly —
// instead of reshuffling every group's assignment, which a fresh
// rand.Perm(len(gs)) would do on any count change.
func groupSortHash(g dedup.Group, seed int64) uint64 {
	ids := append([]string(nil), g.IDs...)
	sort.Strings(ids)
	h := fnv.New64a()
	h.Write([]byte(strconv.FormatInt(seed, 10)))
	h.Write([]byte("|"))
	h.Write([]byte(strings.Join(ids, ",")))
	return h.Sum64()
}
