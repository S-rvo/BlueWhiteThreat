package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssistantLabelsKeepOriginalEvaluationLabel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "labels.json")
	b := fmt.Sprintf(`{"documents":[{"id":"a","text_sha256":"%x","original_human_label":0,"proposed_label":1,"reviewer":"assistant","human_validated":false}]}`, sha256.Sum256([]byte("text")))
	if err := os.WriteFile(path, []byte(b), 0600); err != nil {
		t.Fatal(err)
	}
	labels, hash, err := loadAssistantLabels(path, []string{"a"}, []string{"text"}, []float64{0})
	if err != nil || hash == "" {
		t.Fatalf("load: %v", err)
	}
	g := group{id: "a", label: 0}
	if foldTrainingLabel(g, labels) != 1 || g.label != 0 || foldTrainingLabel(g, nil) != 0 {
		t.Fatal("original evaluation label changed")
	}
	if _, _, err := loadAssistantLabels(path, []string{"b"}, []string{"text"}, []float64{0}); err == nil {
		t.Fatal("accepted ID outside train")
	}
	if _, _, err := loadAssistantLabels(path, []string{"a"}, []string{"altered"}, []float64{0}); err == nil {
		t.Fatal("accepted changed text")
	}
	if _, _, err := loadAssistantLabels(path, []string{"a"}, []string{"text"}, []float64{1}); err == nil {
		t.Fatal("accepted changed original label")
	}
}

func TestAssistantAbstentionsRequireExplicitNullAndPreserveOriginalLabels(t *testing.T) {
	path := filepath.Join(t.TempDir(), "review.json")
	body := fmt.Sprintf(`{"documents":[{"id":"duplicate","text_sha256":"%x","original_human_label":1,"proposed_label":null,"reviewer":"assistant","human_validated":false}]}`, sha256.Sum256([]byte("text")))
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	labels, abstentions, hash, err := loadAssistantReview(path, []string{"duplicate"}, []string{"text"}, []float64{1})
	if err != nil || hash == "" || len(labels) != 0 || !abstentions["duplicate"] {
		t.Fatalf("review: %v, labels=%v abstentions=%v", err, labels, abstentions)
	}
	g := group{id: "representative", members: []string{"representative", "duplicate"}, label: 1}
	if !abstainedGroup(g, abstentions) || abstainedGroup(group{id: "other"}, abstentions) || abstainedGroup(g, nil) {
		t.Fatal("abstention did not follow entire duplicate group")
	}
	if foldTrainingLabel(g, labels) != 1 || g.label != 1 {
		t.Fatal("abstention changed an original label")
	}
	missing := strings.Replace(body, `"proposed_label":null,`, "", 1)
	if err := os.WriteFile(path, []byte(missing), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := loadAssistantReview(path, []string{"duplicate"}, []string{"text"}, []float64{1}); err == nil {
		t.Fatal("missing decision interpreted as explicit abstention")
	}
}
