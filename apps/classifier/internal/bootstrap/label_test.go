// apps/classifier/internal/bootstrap/label_test.go
package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLabelByKeywords(t *testing.T) {
	keywords := []string{"ransomware", "leaked", "exploit"}

	cases := []struct {
		name string
		text string
		want int
	}{
		{
			name: "two keyword hits labels relevant",
			text: "A new ransomware strain has leaked victim data online.",
			want: 1,
		},
		{
			name: "single keyword hit labels not relevant",
			text: "Researchers found a minor ransomware mention in passing.",
			want: 0,
		},
		{
			name: "no keyword hits labels not relevant",
			text: "The quarterly earnings report exceeded expectations.",
			want: 0,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := LabelByKeywords(c.text, keywords)
			if got != c.want {
				t.Fatalf("LabelByKeywords(%q) = %d, want %d", c.text, got, c.want)
			}
		})
	}
}

func TestLoadKeywords(t *testing.T) {
	keywords, err := LoadKeywords("../../data/seed_keywords.json")
	if err != nil {
		t.Fatalf("LoadKeywords failed: %v", err)
	}
	if len(keywords) == 0 {
		t.Fatal("expected a non-empty keyword list")
	}
}

func TestLoadKeywordsNonexistentFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.json")

	keywords, err := LoadKeywords(path)
	if err == nil {
		t.Fatal("LoadKeywords() on nonexistent file returned nil error, want non-nil")
	}
	if keywords != nil {
		t.Fatalf("LoadKeywords() on nonexistent file returned non-nil keywords: %+v", keywords)
	}
}

func TestLoadKeywordsInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(path, []byte("not valid json{{{"), 0o644); err != nil {
		t.Fatalf("failed to set up test fixture: %v", err)
	}

	keywords, err := LoadKeywords(path)
	if err == nil {
		t.Fatal("LoadKeywords() on invalid JSON returned nil error, want non-nil")
	}
	if keywords != nil {
		t.Fatalf("LoadKeywords() on invalid JSON returned non-nil keywords: %+v", keywords)
	}
}
