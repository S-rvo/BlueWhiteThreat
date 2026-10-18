package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestCombinedVectorsNormalizeWithoutMutatingInputs(t *testing.T) {
	a, b := []float64{1, 0}, []float64{0, 1}
	got := combineVectors(a, b)
	if len(got) != 4 || math.Abs(got[0]-1/math.Sqrt(2)) > 1e-12 || got[3] != got[0] {
		t.Fatalf("wrong combined vector: %v", got)
	}
	if a[0] != 1 || b[1] != 1 {
		t.Fatal("inputs modified")
	}
	got = combineVectors([]float64{0, 0}, b)
	if got[3] != 1 {
		t.Fatal("empty lexical block must retain normalized semantic block")
	}
}

func TestEmbeddingCacheRejectsWrongTextAndCoverage(t *testing.T) {
	dir := t.TempDir()
	groups := []group{{id: "a", text: "é漢🙂 text"}}
	protocol, _ := json.Marshal(map[string]any{"dimensions": 768, "truncate": false, "block_characters": 1000, "representative_ids": []string{"a"}})
	if err := os.WriteFile(filepath.Join(dir, "protocol.json"), protocol, 0600); err != nil {
		t.Fatal(err)
	}
	sha := fmt.Sprintf("%x", sha256.Sum256(protocol))
	vector := make([]float64, 768)
	vector[0] = 1
	row := map[string]any{"id": "a", "text_sha256": fmt.Sprintf("%x", sha256.Sum256([]byte(groups[0].text))), "protocol_sha256": sha, "characters": len([]rune(groups[0].text)), "chunks": 1, "vector": vector}
	path := filepath.Join(dir, "vectors.json")
	save := func() {
		raw, _ := json.Marshal(map[string]any{"protocol_sha256": sha, "rows": []any{row}})
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	save()
	got, _, err := loadEmbeddings(path, groups)
	if err != nil || got["a"][0] != 1 {
		t.Fatalf("valid cache: %v", err)
	}
	for key, bad := range map[string]any{"text_sha256": "wrong", "protocol_sha256": "wrong", "chunks": 2, "characters": 99, "vector": []float64{1}} {
		old := row[key]
		row[key] = bad
		save()
		if _, _, err := loadEmbeddings(path, groups); err == nil {
			t.Fatalf("accepted corrupt %s", key)
		}
		row[key] = old
	}
}
