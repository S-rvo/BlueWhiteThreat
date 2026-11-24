package main

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizeKeepsTextButNotMetadataShortcut(t *testing.T) {
	v := victim{PostTitle: "Example Corp", Description: strings.Repeat("Files and internal records were published after a ransomware incident. ", 3), Discovered: "2026-10-01T00:00:00Z", Group: "example-group", Country: "US", Activity: "Technology"}
	d := normalize(v)
	if d == nil {
		t.Fatal("expected document")
	}
	if strings.Contains(d.Text, v.Group) || strings.Contains(d.Text, v.Country) || d.LabelAuthority == "" || d.SourceActor != v.Group {
		t.Fatalf("metadata leaked into text: %+v", d)
	}
}

func TestNormalizeRejectsTerseRowsAndBoundsMonths(t *testing.T) {
	if normalize(victim{PostTitle: "Example", Description: "N/A", Discovered: "2026-10-01"}) != nil {
		t.Fatal("terse row retained")
	}
	start, _ := time.Parse("2006-01", "2025-01")
	end, _ := time.Parse("2006-01", "2027-02")
	if _, err := months(start, end); err == nil {
		t.Fatal("unbounded date range accepted")
	}
}

func TestRetentionBoundCoversFullRecentArchiveWindow(t *testing.T) {
	if maxRetained < 20000 {
		t.Fatalf("retention cap %d would truncate the bounded archive window", maxRetained)
	}
}
