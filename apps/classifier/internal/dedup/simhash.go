// apps/classifier/internal/dedup/simhash.go
package dedup

import (
	"hash/fnv"
	"math/bits"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/text"
)

// simHashBits is the width of the fingerprint. 64 bits gives a workable
// hamming-distance resolution for the Normalize-based grouping this
// package already does; near-duplicate detection only needs to rank
// documents by similarity, not reconstruct them.
const simHashBits = 64

// simHash computes a 64-bit SimHash fingerprint of doc's word tokens:
// each token is hashed to 64 bits, and each bit position accumulates +1 or
// -1 across all tokens depending on whether that token's hash has the bit
// set; the fingerprint's bit is 1 wherever the accumulated vote is
// positive. Unlike Normalize's exact-match grouping, two documents that
// differ by a few words (e.g. a scraped page re-crawled with minor edits,
// or a paraphrased repost) produce fingerprints that differ in only a
// handful of bits, measurable via hammingDistance.
func simHash(doc string) uint64 {
	var votes [simHashBits]int
	for _, tok := range text.Tokenize(doc) {
		h := fnv.New64a()
		h.Write([]byte(tok))
		sum := h.Sum64()
		for bit := 0; bit < simHashBits; bit++ {
			if sum&(1<<uint(bit)) != 0 {
				votes[bit]++
			} else {
				votes[bit]--
			}
		}
	}

	var fingerprint uint64
	for bit := 0; bit < simHashBits; bit++ {
		if votes[bit] > 0 {
			fingerprint |= 1 << uint(bit)
		}
	}
	return fingerprint
}

// hammingDistance returns the number of differing bits between a and b.
func hammingDistance(a, b uint64) int {
	return bits.OnesCount64(a ^ b)
}

// MergeNearDuplicates clusters groups whose representative text (the
// first text in the group) has a SimHash within maxDistance hamming bits
// of another group's, treating them as the same underlying content even
// though Normalize didn't produce an exact match (e.g. a re-crawl with
// minor copy edits, or a repost with light paraphrasing). Clusters where
// every member shares the same Label are merged into a single Group;
// clusters where members disagree on Label are reported as a Conflict
// instead, exactly like GroupByNormalizedText does for exact duplicates,
// so contradictory labels are surfaced rather than guessed at. Groups are
// processed in their given order and merged into the first cluster they're
// close enough to, so result order is deterministic given a deterministic
// input order.
func MergeNearDuplicates(groups []Group, maxDistance int) (merged []Group, conflicts []Conflict) {
	type cluster struct {
		fingerprint uint64
		idxs        []int
	}
	var clusters []cluster

	fingerprints := make([]uint64, len(groups))
	for i, g := range groups {
		fingerprints[i] = simHash(g.Texts[0])

		placed := false
		for ci := range clusters {
			if hammingDistance(fingerprints[i], clusters[ci].fingerprint) <= maxDistance {
				clusters[ci].idxs = append(clusters[ci].idxs, i)
				placed = true
				break
			}
		}
		if !placed {
			clusters = append(clusters, cluster{fingerprint: fingerprints[i], idxs: []int{i}})
		}
	}

	for _, c := range clusters {
		if len(c.idxs) == 1 {
			merged = append(merged, groups[c.idxs[0]])
			continue
		}

		consistent := true
		label := groups[c.idxs[0]].Label
		for _, idx := range c.idxs {
			if groups[idx].Label != label {
				consistent = false
				break
			}
		}

		if consistent {
			g := Group{Key: groups[c.idxs[0]].Key, Label: label}
			for _, idx := range c.idxs {
				g.IDs = append(g.IDs, groups[idx].IDs...)
				g.Texts = append(g.Texts, groups[idx].Texts...)
			}
			merged = append(merged, g)
		} else {
			conflict := Conflict{Key: groups[c.idxs[0]].Key}
			for _, idx := range c.idxs {
				conflict.IDs = append(conflict.IDs, groups[idx].IDs...)
				conflict.Texts = append(conflict.Texts, groups[idx].Texts...)
				for range groups[idx].IDs {
					conflict.Labels = append(conflict.Labels, groups[idx].Label)
				}
			}
			conflicts = append(conflicts, conflict)
		}
	}
	return merged, conflicts
}
