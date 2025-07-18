package scraper_tests

import (
	"os"
	"strings"
	"testing"

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/deepdarkCTI"
)

func TestGetFilesToScrape_Default(t *testing.T) {
    os.Unsetenv("SCRAP_FILES")
    files := deepdarkCTI.GetFilesToScrape()
    if len(files) < 5 {
        t.Errorf("Default file list trop courte: %#v", files)
    }
}

func TestGetFilesToScrape_Custom(t *testing.T) {
    os.Setenv("SCRAP_FILES", "foo.md,bar.md")
    files := deepdarkCTI.GetFilesToScrape()
    if len(files) != 2 || files[0] != "foo.md" || files[1] != "bar.md" {
        t.Errorf("List custom bad: %#v", files)
    }
}

func TestExtractNameAndUrl(t *testing.T) {
    name, url := deepdarkCTI.ExtractNameAndUrl("[truc](http://test)")
    if name != "truc" || url != "http://test" {
        t.Errorf("Parsing error got=%q,%q", name, url)
    }
}

func TestParseMarkdownColumns(t *testing.T) {
    columns := deepdarkCTI.ParseMarkdownColumns("|foo|bar| |baz|")
    if len(columns) != 4 || columns[2] != "" {
        t.Errorf("Columns parsing bad: %#v", columns)
    }
}

func TestParsePatch_AllAdded(t *testing.T) {
    patch := `
+|foo|bar|baz|
-|bidule|xx|nope|`
    added, removed := deepdarkCTI.ParsePatch(patch)
    if len(added) != 1 || !strings.Contains(added[0], "foo") {
        t.Errorf("Added lines incorrect: %#v", added)
    }
    if len(removed) != 1 || !strings.Contains(removed[0], "bidule") {
        t.Errorf("Removed lines incorrect: %#v", removed)
    }
}
