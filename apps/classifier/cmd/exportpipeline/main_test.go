package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/db"
)

func TestPipelineDocumentUsesStableTypeAndNoVerdict(t *testing.T) {
	d, ok := pipelineDocument(db.ArticleContent{ID: "mongo-id", URL: "http://example.onion/post/1", Source: "http://example.onion/post/1", Content: "incident text", ScrapedAt: time.Date(2026, 10, 20, 1, 2, 3, 0, time.UTC)})
	if !ok || d.Source != "darkweb_page" || d.SourceType != "darkweb_page" || d.Group != "host:example.onion" {
		t.Fatalf("unexpected document: %+v, ok=%v", d, ok)
	}
	if d.Text != "incident text" || d.CollectedAt == "" || d.ID == "" {
		t.Fatalf("missing frozen content metadata: %+v", d)
	}
}

func TestReadExcludedNormalized(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.jsonl")
	if err := os.WriteFile(path, []byte("{\"text\":\"  Same  TEXT \\n\"}\n{\"text\":\"other\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := readExcludedNormalized(path)
	if err != nil {
		t.Fatal(err)
	}
	if !got["same text"] || !got["other"] || len(got) != 2 {
		t.Fatalf("unexpected exclusion set: %#v", got)
	}
}

func TestPipelineDocumentExcludesEmptyAndDirectory(t *testing.T) {
	if _, ok := pipelineDocument(db.ArticleContent{}); ok {
		t.Fatal("empty article exported")
	}
}
