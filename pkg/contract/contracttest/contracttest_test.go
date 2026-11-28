package contracttest

import (
	"os"
	"path/filepath"
	"testing"
)

type sample struct {
	ID      any    `bson:"_id"`
	URL     string `bson:"url,omitempty"`
	Skipped string `bson:"-"`
	private string
}

func TestBSONTags(t *testing.T) {
	_ = sample{}.private
	got := BSONTags(&sample{})
	if len(got) != 2 || got[0] != "_id" || got[1] != "url" {
		t.Fatalf("BSONTags = %v", got)
	}
}

func TestKeyLiteralsSeesLuaAndSkipsTests(t *testing.T) {
	dir := t.TempDir()
	prod := "package x\nconst s = `redis.call('SADD','urls_crawled',u)`\nvar k = \"crawler:hosts\"\n"
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte(prod), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a_test.go"), []byte("package x\nvar k = \"crawler:nope\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := KeyLiterals(t, dir, func(s string) bool { return len(s) > 7 && (s[:8] == "crawler:" || s[:5] == "urls_") })
	keys := got[filepath.Join(dir, "a.go")]
	if len(got) != 1 || len(keys) != 2 {
		t.Fatalf("KeyLiterals = %v", got)
	}
}

func TestIsSharedPrefix(t *testing.T) {
	shared := []string{"crawler:scheduled_host:", "urls_crawled"}
	for key, want := range map[string]bool{
		"crawler:scheduled_host:": true, "crawler:scheduled_host": true,
		"crawler:scheduled_host:x.onion": true, "urls_crawled": true, "urls_crawled_at": false,
	} {
		if got := isShared(key, shared); got != want {
			t.Errorf("isShared(%q) = %v", key, got)
		}
	}
}

type recorder struct {
	testing.TB
	errors int
}

func (r *recorder) Errorf(string, ...any) { r.errors++ }

func TestRequireKeysReferenced(t *testing.T) {
	dir := t.TempDir()
	prod := "package x\nconst s = `redis.call('SADD','urls_scraped',u)`\n"
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte(prod), 0o600); err != nil {
		t.Fatal(err)
	}
	rec := &recorder{TB: t}
	RequireKeysReferenced(rec, dir, []string{"urls_scraped", "urls_renamed"})
	if rec.errors != 1 {
		t.Fatalf("erreurs = %d, attendu 1 (urls_renamed absente)", rec.errors)
	}
}
