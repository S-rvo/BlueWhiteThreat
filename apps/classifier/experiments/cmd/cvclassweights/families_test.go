package main

import (
	"reflect"
	"testing"
)

func TestConstrainFamilies(t *testing.T) {
	groups := []group{{id: "c"}, {id: "a"}, {id: "b"}, {id: "d"}}
	for _, tc := range []struct {
		name     string
		families [][]string
		want     []int
		bad      bool
	}{
		{"unchanged", nil, []int{2, 4, 1, 0}, false},
		{"transitive", [][]string{{"c", "b"}, {"a", "b"}}, []int{4, 4, 4, 0}, false},
		{"unknown", [][]string{{"a", "missing"}}, []int{2, 4, 1, 0}, true},
		{"duplicate", [][]string{{"a", "a"}}, []int{2, 4, 1, 0}, true},
		{"singleton", [][]string{{"a"}}, []int{2, 4, 1, 0}, true},
		{"atomic", [][]string{{"a", "b"}, {"c", "missing"}}, []int{2, 4, 1, 0}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assigned := []int{2, 4, 1, 0}
			err := constrainFamilies(groups, assigned, tc.families)
			if (err != nil) != tc.bad {
				t.Fatalf("error: %v", err)
			}
			if !reflect.DeepEqual(assigned, tc.want) {
				t.Fatalf("folds %v", assigned)
			}
		})
	}
}
