package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHistoricalReferencesReserveBlankIDsAndRejectContradictions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reference.csv")
	if err := os.WriteFile(path, []byte("id,text,label\na,,0\nb,some text,1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	docs, err := readReference(path)
	if err != nil || len(docs) != 2 || docs[0].ID != "a" || docs[0].Text != "" {
		t.Fatalf("lost historical reference: %v", err)
	}
	for _, bad := range []string{"id,text,label\na,one,0\na,two,1\n", "id,text,label\na,something,unknown\n"} {
		if err = os.WriteFile(path, []byte(bad), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err = readReference(path); err == nil {
			t.Fatal("invalid references accepted")
		}
	}
}
