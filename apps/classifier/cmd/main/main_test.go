// apps/classifier/cmd/main/main_test.go
package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/classify"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/db"
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

func TestApplyThresholdOverrideCreatesStableDecisionVersion(t *testing.T) {
	first := &model.Model{Threshold: 0.5, Version: "model-v1"}
	if !applyThresholdOverride(first, "0.7") {
		t.Fatal("expected a changed threshold to be applied")
	}
	if first.Threshold != 0.7 || first.Version != "model-v1-threshold-0.7" {
		t.Fatalf("effective model = threshold %v, version %q", first.Threshold, first.Version)
	}

	second := &model.Model{Threshold: 0.5, Version: "model-v1"}
	applyThresholdOverride(second, "0.7")
	if second.Version != first.Version {
		t.Fatalf("same override produced unstable versions %q and %q", first.Version, second.Version)
	}
}

func TestApplyThresholdOverrideIgnoresInvalidAndUnchangedValues(t *testing.T) {
	for _, raw := range []string{"", "not-a-number", "-0.1", "1.1", "NaN", "+Inf"} {
		m := &model.Model{Threshold: 0.5, Version: "model-v1"}
		if applyThresholdOverride(m, raw) {
			t.Errorf("applyThresholdOverride(%q) unexpectedly changed the model", raw)
		}
		if m.Threshold != 0.5 || m.Version != "model-v1" {
			t.Errorf("invalid override %q changed the model: %#v", raw, m)
		}
	}
	m := &model.Model{Threshold: 0.5, Version: "model-v1"}
	if applyThresholdOverride(m, "0.5") {
		t.Fatal("unchanged threshold should not create a new inference version")
	}
}

func TestLoadClassifierAppliesThresholdOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model.json")
	input := &model.Model{
		Vocabulary: map[string]int{"ransomware": 0},
		IDF:        []float64{1},
		Weights:    []float64{2},
		Threshold:  0.5,
		Version:    "model-v1",
	}
	if err := model.Save(path, input); err != nil {
		t.Fatalf("save model fixture: %v", err)
	}

	classifier, err := loadClassifier(path, "0.7")
	if err != nil {
		t.Fatalf("loadClassifier() error = %v", err)
	}
	if classifier.Version() != "model-v1-threshold-0.7" {
		t.Fatalf("classifier version = %q", classifier.Version())
	}
}

func TestLoadClassifierVersionsLegacyModelFromFileContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-model.json")
	input := &model.Model{
		Vocabulary: map[string]int{"ransomware": 0},
		IDF:        []float64{1},
		Weights:    []float64{2},
		Threshold:  0.5,
	}
	if err := model.Save(path, input); err != nil {
		t.Fatalf("save model fixture: %v", err)
	}

	classifier, err := loadClassifier(path, "")
	if err != nil {
		t.Fatalf("loadClassifier() error = %v", err)
	}
	if !strings.HasPrefix(classifier.Version(), "legacy-") {
		t.Fatalf("classifier version = %q, want legacy content hash", classifier.Version())
	}
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

func TestClassifyArticlePersistsPolicyAndZeroVectorReview(t *testing.T) {
	for _, content := range []string{"unseen vocabulary", "ransomware", strings.Repeat("link.onion ", 20)} {
		r, skip := classifyArticle(testClassifier(t), content)
		if skip || r.PolicyVersion != db.ClassificationPolicyVersion {
			t.Fatalf("content %q: skip=%v policy=%q", content, skip, r.PolicyVersion)
		}
		if content == "unseen vocabulary" && !r.NeedsReview {
			t.Fatal("zero-vector review was lost before storage")
		}
		if content == "unseen vocabulary" && r.ReviewReason != classify.ReviewNoFeatures {
			t.Fatalf("zero-vector reason lost before storage: %q", r.ReviewReason)
		}
	}
}
