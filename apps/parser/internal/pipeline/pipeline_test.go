package pipeline

import (
	"context"
	"errors"
	"testing"

	"github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/entities"
)

type fakeExtractor struct {
	result entities.Entities
	err    error
	calls  int
}

func (f *fakeExtractor) Extract(ctx context.Context, text string) (entities.Entities, error) {
	f.calls++
	return f.result, f.err
}

func TestExtractWithFallback_UsesPrimaryWhenSuccessful(t *testing.T) {
	primary := &fakeExtractor{}
	primary.result.CVEs = []string{"CVE-2024-1234"}
	fallback := &fakeExtractor{}

	out, err := ExtractWithFallback(context.Background(), primary, fallback, "text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.CVEs) != 1 {
		t.Fatalf("expected primary result, got %v", out)
	}
	if fallback.calls != 0 {
		t.Fatal("fallback should not be called when primary succeeds")
	}
}

func TestExtractWithFallback_EscalatesOnPrimaryError(t *testing.T) {
	primary := &fakeExtractor{err: errors.New("ollama down")}
	fallback := &fakeExtractor{}
	fallback.result.Malware = []string{"Emotet"}

	out, err := ExtractWithFallback(context.Background(), primary, fallback, "text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Malware) != 1 || fallback.calls != 1 {
		t.Fatalf("expected fallback used, got out=%v calls=%d", out, fallback.calls)
	}
}

func TestExtractWithFallback_EscalatesOnEmptyResult(t *testing.T) {
	primary := &fakeExtractor{} // résultat vide, pas d'erreur
	fallback := &fakeExtractor{}
	fallback.result.CVEs = []string{"CVE-2024-9999"}

	out, err := ExtractWithFallback(context.Background(), primary, fallback, "text mentioning CVE-2024-9999")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.CVEs) != 1 || fallback.calls != 1 {
		t.Fatalf("expected fallback used on empty primary result, got out=%v calls=%d", out, fallback.calls)
	}
}

func TestExtractWithFallback_BothFail(t *testing.T) {
	primary := &fakeExtractor{err: errors.New("ollama down")}
	fallback := &fakeExtractor{err: errors.New("claude down")}

	_, err := ExtractWithFallback(context.Background(), primary, fallback, "text")
	if err == nil {
		t.Fatal("expected error when both extractors fail")
	}
}
