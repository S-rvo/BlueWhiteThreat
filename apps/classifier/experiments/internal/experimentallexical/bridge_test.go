package experimentallexical

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func fixture(t *testing.T, body string) Bridge {
	t.Helper()
	p := filepath.Join(t.TempDir(), "scorer.sh")
	if e := os.WriteFile(p, []byte("#!/bin/sh\ncat >/dev/null\n"+body+"\n"), 0600); e != nil {
		t.Fatal(e)
	}
	return Bridge{Python: "/bin/sh", Script: p, Candidate: "fixture"}
}
func TestBridgeContract(t *testing.T) {
	tests := []struct {
		name, response string
		valid          bool
	}{
		{"scored", `{"id":"a","score":0.8,"relevant":true,"threshold":0.5,"status":"scored","version":"test"}`, true},
		{"abstain", `{"id":"a","score":0.8,"relevant":null,"threshold":null,"status":"unknown_source_abstention","version":"test"}`, true},
		{"wrong-id", `{"id":"b","score":0.8,"relevant":true,"threshold":0.5,"status":"scored","version":"test"}`, false},
		{"wrong-verdict", `{"id":"a","score":0.8,"relevant":false,"threshold":0.5,"status":"scored","version":"test"}`, false},
		{"out-of-range", `{"id":"a","score":8,"status":"unknown_source_abstention","version":"test"}`, false},
		{"malformed", `not-json`, false},
		{"missing-result", ``, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := fixture(t, "printf '%s\\n' '"+tt.response+"'")
			_, err := b.Score(context.Background(), []Document{{ID: "a", Text: "sample"}})
			if (err == nil) != tt.valid {
				t.Fatalf("valid=%v err=%v", tt.valid, err)
			}
		})
	}
}
func TestBridgeSubprocessFailure(t *testing.T) {
	b := fixture(t, "exit 2")
	if _, err := b.Score(context.Background(), []Document{{ID: "a"}}); err == nil {
		t.Fatal("expected subprocess error")
	}
}
func TestBridgeEmptyBatch(t *testing.T) {
	r, err := (Bridge{}).Score(context.Background(), nil)
	if err != nil || len(r) != 0 {
		t.Fatal("unexpected empty-batch result")
	}
}
func TestBridgeCancelledContext(t *testing.T) {
	b := fixture(t, "sleep 30")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := b.Score(ctx, []Document{{ID: "a"}}); err == nil {
		t.Fatal("expected cancellation")
	}
}
