package dedup

import (
	"sort"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/overlap"
)

type MergeDiagnostics struct {
	Components                        int        `json:"components"`
	MultiDocumentComponents           int        `json:"multi_document_components"`
	LargestComponentDocuments         int        `json:"largest_component_documents"`
	TransitiveOnlyComponents          int        `json:"transitive_only_components"`
	LargestComponentIDs               []string   `json:"largest_component_ids,omitempty"`
	TransitiveComponentIDs            [][]string `json:"transitive_component_ids,omitempty"`
	LargestComponentMinimumSimilarity float64    `json:"largest_component_minimum_similarity"`
	MaximumComponentDiameter          float64    `json:"maximum_component_diameter"`
}

// MergeContentDuplicates merges only normalized-exact or strongly overlapping
// documents. Unlike lexical SimHash, this requires 80% Jaccard overlap between
// five-token shingles (and at least 20 tokens), so documents that merely share
// a topic or writing style are not collapsed.
func MergeContentDuplicates(groups []Group) (merged []Group, conflicts []Conflict) {
	merged, conflicts, _ = MergeContentDuplicatesWithDiagnostics(groups)
	return merged, conflicts
}

// MergeContentDuplicatesWithDiagnostics also reports whether connected
// components rely on transitive links between non-similar endpoints.
func MergeContentDuplicatesWithDiagnostics(groups []Group) (merged []Group, conflicts []Conflict, diagnostics MergeDiagnostics) {
	prints := make([]overlap.Print, len(groups))
	parent := make([]int, len(groups))
	for i, g := range groups {
		prints[i] = overlap.Fingerprint(g.Texts[0])
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		if parent[i] != i {
			parent[i] = find(parent[i])
		}
		return parent[i]
	}
	index := map[string][]int{}
	for i := range groups {
		candidates := map[int]struct{}{}
		keys := overlap.CandidateKeys(prints[i])
		for _, key := range keys {
			for _, j := range index[key] {
				candidates[j] = struct{}{}
			}
		}
		orderedCandidates := make([]int, 0, len(candidates))
		for j := range candidates {
			orderedCandidates = append(orderedCandidates, j)
		}
		sort.Ints(orderedCandidates)
		for _, j := range orderedCandidates {
			if overlap.Similar(prints[i], prints[j]) {
				a, b := find(i), find(j)
				if a != b {
					if a < b {
						parent[b] = a
					} else {
						parent[a] = b
					}
				}
			}
		}
		for _, key := range keys {
			index[key] = append(index[key], i)
		}
	}
	clusters := map[int][]int{}
	var clusterOrder []int
	for i := range groups {
		root := find(i)
		if _, ok := clusters[root]; !ok {
			clusterOrder = append(clusterOrder, root)
		}
		clusters[root] = append(clusters[root], i)
	}
	for _, root := range clusterOrder {
		idxs := clusters[root]
		diagnostics.Components++
		if len(idxs) > diagnostics.LargestComponentDocuments {
			diagnostics.LargestComponentDocuments = len(idxs)
			diagnostics.LargestComponentIDs = componentIDs(groups, idxs)
			diagnostics.LargestComponentMinimumSimilarity = componentMinimumSimilarity(prints, idxs)
		}
		if len(idxs) > 1 {
			minimumSimilarity := componentMinimumSimilarity(prints, idxs)
			if diameter := 1 - minimumSimilarity; diameter > diagnostics.MaximumComponentDiameter {
				diagnostics.MaximumComponentDiameter = diameter
			}
			diagnostics.MultiDocumentComponents++
			allPairsSimilar := true
			for i := 0; i < len(idxs) && allPairsSimilar; i++ {
				for j := 0; j < i; j++ {
					if !overlap.Similar(prints[idxs[i]], prints[idxs[j]]) {
						allPairsSimilar = false
						break
					}
				}
			}
			if !allPairsSimilar {
				diagnostics.TransitiveOnlyComponents++
				diagnostics.TransitiveComponentIDs = append(diagnostics.TransitiveComponentIDs, componentIDs(groups, idxs))
			}
		}
		if len(idxs) == 1 {
			merged = append(merged, groups[idxs[0]])
			continue
		}
		label := groups[idxs[0]].Label
		consistent := true
		for _, idx := range idxs[1:] {
			if groups[idx].Label != label {
				consistent = false
				break
			}
		}
		if consistent {
			g := Group{Key: groups[idxs[0]].Key, Label: label}
			for _, idx := range idxs {
				g.IDs = append(g.IDs, groups[idx].IDs...)
				g.Texts = append(g.Texts, groups[idx].Texts...)
			}
			merged = append(merged, g)
			continue
		}
		conflict := Conflict{Key: groups[idxs[0]].Key}
		for _, idx := range idxs {
			conflict.IDs = append(conflict.IDs, groups[idx].IDs...)
			conflict.Texts = append(conflict.Texts, groups[idx].Texts...)
			for range groups[idx].IDs {
				conflict.Labels = append(conflict.Labels, groups[idx].Label)
			}
		}
		conflicts = append(conflicts, conflict)
	}
	return merged, conflicts, diagnostics
}

func componentMinimumSimilarity(prints []overlap.Print, indices []int) float64 {
	if len(indices) < 2 {
		return 1
	}
	minimum := 1.0
	for i := 0; i < len(indices); i++ {
		for j := 0; j < i; j++ {
			if similarity := overlap.Similarity(prints[indices[i]], prints[indices[j]]); similarity < minimum {
				minimum = similarity
			}
		}
	}
	return minimum
}

func componentIDs(groups []Group, indices []int) []string {
	var ids []string
	for _, index := range indices {
		ids = append(ids, groups[index].IDs...)
	}
	sort.Strings(ids)
	return ids
}
