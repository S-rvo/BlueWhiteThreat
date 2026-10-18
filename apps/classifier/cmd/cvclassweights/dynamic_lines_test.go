package main

import (
	"fmt"
	"strings"
	"testing"
)

func interfaceText() string {
	var lines []string
	for i := 0; i < 44; i++ {
		lines = append(lines, fmt.Sprintf("Interface field %d", i))
	}
	return strings.Join(lines, "\n")
}

func TestDynamicLinesGroupsSmallChangingRegions(t *testing.T) {
	a := fingerprintLines(interfaceText() + "\nVisitors: alpha beta")
	b := fingerprintLines(interfaceText() + "\nVisitors: gamma delta")
	if !similarLines(a, b) || !similarLines(b, a) {
		t.Fatal("changing visitor list should remain in the same interface group")
	}
}

func TestDynamicLinesRejectsArticlesSharingNavigation(t *testing.T) {
	// Similar line counts alone must not merge different long articles.
	a := fingerprintLines(interfaceText() + "\n" + strings.Repeat("incident-a details ", 200))
	b := fingerprintLines(interfaceText() + "\n" + strings.Repeat("incident-b details ", 200))
	if similarLines(a, b) {
		t.Fatal("shared navigation dominated the article comparison")
	}
	if similarLines(fingerprintLines("short\ninterface"), fingerprintLines("short\ninterface")) {
		t.Fatal("short forms must use existing exact-text grouping")
	}
}

func TestLineFingerprintNormalizationAndDistinctCounts(t *testing.T) {
	p := fingerprintLines(" FIELD One \nfield   one\n\nSecond line")
	if len(p.lines) != 2 || p.words != 4 {
		t.Fatalf("unexpected distinct lines: %+v", p)
	}
}
