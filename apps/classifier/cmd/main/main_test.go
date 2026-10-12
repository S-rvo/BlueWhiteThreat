// apps/classifier/cmd/main/main_test.go
package main

import (
	"strings"
	"testing"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/classify"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/model"
)

func testClassifier(t *testing.T) *classify.Classifier {
	t.Helper()
	m := &model.Model{
		Vocabulary: map[string]int{"ransomware": 0},
		IDF:        []float64{1.0},
		Weights:    []float64{5.0},
		Bias:       -1.0,
		Threshold:  0.5,
		Version:    "unit-test",
	}
	return classify.New(m)
}

func TestClassifyArticle_EmptyContentSkips(t *testing.T) {
	_, skip := classifyArticle(testClassifier(t), "")
	if !skip {
		t.Fatal("expected skip=true for empty content")
	}
}

func TestClassifyArticle_LinkDirectorySkipsScoringAndIsMarkedNotRelevant(t *testing.T) {
	// A link-directory page (many .onion mentions) is excluded from
	// cmd/bootstrap and cmd/export's training data — the live classifier
	// never saw one as a labeled example, so it must not score it as if it
	// had, keeping training and inference consistent.
	directoryPage := strings.Repeat("link.onion ", 20)

	relevance, skip := classifyArticle(testClassifier(t), directoryPage)
	if skip {
		t.Fatal("expected skip=false for a directory page: it should be recorded, not retried forever")
	}
	if relevance.Relevant {
		t.Fatal("expected a directory page to be recorded as not relevant")
	}
	if relevance.ModelVersion != "unit-test" {
		t.Fatalf("expected model version to be propagated, got %q", relevance.ModelVersion)
	}
}

func TestClassifyArticle_OrdinaryContentIsScoredNormally(t *testing.T) {
	relevance, skip := classifyArticle(testClassifier(t), "a new ransomware campaign is spreading")
	if skip {
		t.Fatal("expected skip=false for ordinary content")
	}
	if !relevance.Relevant {
		t.Fatalf("expected relevant=true, got score=%f", relevance.Score)
	}
	if relevance.NeedsReview {
		t.Fatalf("expected needs_review=false for a confident score=%f", relevance.Score)
	}
}
