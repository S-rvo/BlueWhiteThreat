package review

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReviewed(t *testing.T) {
	for _, tt := range []struct {
		name, csv string
		want      Label
		fail      bool
	}{
		{"legacy human", "id,label\na,1\n", Label{1, SourceHuman}, false},
		{"explicit human", "id,label,source\na,0,human\n", Label{0, SourceHuman}, false},
		{"LLM", "id,label,source\na,1,llm_reviewed\n", Label{1, SourceLLM}, false},
		{"invalid source", "id,label,source\na,1,robot\n", Label{}, true},
		{"missing provenance", "id,label,source\na,1,\n", Label{}, true},
		{"invalid label", "id,label\na,2\n", Label{}, true},
		{"partial integer", "id,label\na,1junk\n", Label{}, true},
		{"invalid header", "id,label,origin\na,1,human\n", Label{}, true},
		{"truncated row", "id,label,source\na,1\n", Label{}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "labels.csv")
			if err := os.WriteFile(path, []byte(tt.csv), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := LoadReviewed(path)
			if tt.fail {
				if err == nil {
					t.Fatal("expected invalid store error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got["a"] != tt.want {
				t.Fatalf("got %+v, want %+v", got["a"], tt.want)
			}
		})
	}
}

func TestLoadThemeNegatives(t *testing.T) {
	for _, tt := range []struct {
		name, csv string
		wantID    string
		wantTheme string
		wantLen   int
		fail      bool
	}{
		{"single", "id,theme\na,geopolitics\n", "a", "geopolitics", 1, false},
		{"multiple", "id,theme\na,geopolitics\nb,geopolitics\n", "a", "geopolitics", 2, false},
		{"wrong header", "id,reason\na,geopolitics\n", "", "", 0, true},
		{"missing theme", "id,theme\na,\n", "", "", 0, true},
		{"missing id", "id,theme\n,geopolitics\n", "", "", 0, true},
		{"extra column", "id,theme\na,geopolitics,x\n", "", "", 0, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "themes.csv")
			if err := os.WriteFile(path, []byte(tt.csv), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := LoadThemeNegatives(path)
			if tt.fail {
				if err == nil {
					t.Fatal("expected invalid store error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != tt.wantLen {
				t.Fatalf("len = %d, want %d", len(got), tt.wantLen)
			}
			if got[tt.wantID] != tt.wantTheme {
				t.Fatalf("theme for %q = %q, want %q", tt.wantID, got[tt.wantID], tt.wantTheme)
			}
		})
	}
}

func TestLoadThemeNegativesAbsentOrEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "themes.csv")
	for _, empty := range []bool{false, true} {
		if empty {
			if err := os.WriteFile(path, nil, 0600); err != nil {
				t.Fatal(err)
			}
		}
		got, err := LoadThemeNegatives(path)
		if err != nil || len(got) != 0 {
			t.Fatalf("got %v, %v", got, err)
		}
	}
}

func TestLoadReviewedAbsentOrEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "labels.csv")
	for _, empty := range []bool{false, true} {
		if empty {
			if err := os.WriteFile(path, nil, 0600); err != nil {
				t.Fatal(err)
			}
		}
		got, err := LoadReviewed(path)
		if err != nil || len(got) != 0 {
			t.Fatalf("got %v, %v", got, err)
		}
	}
}
