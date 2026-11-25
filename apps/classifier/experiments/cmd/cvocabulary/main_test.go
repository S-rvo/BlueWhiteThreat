package main

import "testing"

func TestGroupedTrainingDropsConflictingNearDuplicates(t *testing.T) {
	base := "A detailed advisory reports several affected software versions and describes a remote vulnerability that may allow an attacker to execute code or cause denial of service. Administrators should apply the security update and review the vendor bulletin for mitigation instructions."
	near := base + " Updated bulletin."
	distinct := "A separate incident report describes a ransomware campaign targeting hospitals with stolen credentials, encrypted files, and a demand for payment in cryptocurrency."
	groups, exactConflicts, nearConflicts := groupedTraining(
		[]string{"a", "b", "c"},
		[]string{base, near, distinct},
		[]float64{1, 0, 1},
	)
	if exactConflicts != 0 {
		t.Fatalf("exact conflicts = %d, want 0", exactConflicts)
	}
	if nearConflicts != 1 {
		t.Fatalf("near conflicts = %d, want 1", nearConflicts)
	}
	if len(groups) != 1 || groups[0].IDs[0] != "c" {
		t.Fatalf("retained groups = %#v, want only the distinct example", groups)
	}
}

func TestAveragePrecisionRanksPositiveFirst(t *testing.T) {
	got := averagePrecision([]float64{0.9, 0.8, 0.1}, []float64{1, 0, 1})
	if got < 0.82 || got > 0.84 {
		t.Fatalf("average precision = %.3f, want 0.833", got)
	}
}
