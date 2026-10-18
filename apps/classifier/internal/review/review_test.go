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
