package main

import "fmt"

// Constraints change folds only, never documents or labels. Validate everything
// before mutating so malformed input cannot partially change the partition.
func constrainFamilies(groups []group, assigned []int, families [][]string) error {
	if len(groups) != len(assigned) {
		return fmt.Errorf("fold assignment length mismatch")
	}
	index := make(map[string]int, len(groups))
	parent := make([]int, len(groups))
	for i, g := range groups {
		index[g.id] = i
		parent[i] = i
	}
	var root func(int) int
	root = func(i int) int {
		if parent[i] != i {
			parent[i] = root(parent[i])
		}
		return parent[i]
	}
	for _, family := range families {
		if len(family) < 2 {
			return fmt.Errorf("family needs at least two representative IDs")
		}
		seen := map[string]bool{}
		first := -1
		for _, id := range family {
			i, ok := index[id]
			if !ok {
				return fmt.Errorf("unknown representative ID %q", id)
			}
			if seen[id] {
				return fmt.Errorf("repeated family ID %q", id)
			}
			seen[id] = true
			if first < 0 {
				first = i
			} else {
				parent[root(i)] = root(first)
			}
		}
	}
	leader := map[int]int{}
	for i, g := range groups {
		r := root(i)
		j, ok := leader[r]
		if !ok || g.id < groups[j].id {
			leader[r] = i
		}
	}
	next := append([]int(nil), assigned...)
	for i := range groups {
		next[i] = assigned[leader[root(i)]]
	}
	copy(assigned, next)
	return nil
}
