package main

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/classify"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/model"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/tfidf"
)

func TestCandidateRetainsRuntimeTransformAfterSerialization(t *testing.T) {
	groups := []group{{id: "a", text: "common benign meeting uniqueword", label: 0}, {id: "b", text: "common benign meeting", label: 0}, {id: "c", text: "common incident ransomware", label: 1}, {id: "d", text: "common incident ransomware", label: 1}}
	m, err := fitFullTrainCandidate(groups, 2, "provisional-test")
	if err != nil {
		t.Fatal(err)
	}
	for term := range m.Vocabulary {
		if strings.Contains(term, "uniqueword") {
			t.Fatal("rare term retained")
		}
	}
	path := filepath.Join(t.TempDir(), "candidate.json")
	if err = model.SaveNew(path, m); err != nil {
		t.Fatal(err)
	}
	loaded, err := model.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, doc := range []string{"common benign meeting", "common incident ransomware", "unknown"} {
		a, b := classify.New(m).Classify(doc), classify.New(loaded).Classify(doc)
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("runtime changed on serialization: %+v %+v", a, b)
		}
	}
	if loaded.Validation != nil {
		t.Fatal("invented validation evidence")
	}
}

func TestHingeCandidateUsesSameObjectiveAndRuntimeTransform(t *testing.T) {
	groups := []group{{id: "a", text: "ordinary meeting benign", label: 0}, {id: "b", text: "ordinary meeting benign", label: 0}, {id: "c", text: "incident malware threat", label: 1}, {id: "d", text: "incident malware threat", label: 1}}
	m, err := fitFullTrainCandidateWithLoss(groups, 2, "provisional-hinge-test", "squared-hinge")
	if err != nil {
		t.Fatal(err)
	}
	texts, labels := []string{}, []float64{}
	for _, g := range groups {
		texts = append(texts, g.text)
		labels = append(labels, g.label)
	}
	v, err := tfidf.FitWithMinDocumentFrequency(texts, labels, 2)
	if err != nil {
		t.Fatal(err)
	}
	x := make([][]float64, len(texts))
	for i, text := range texts {
		x[i] = v.Transform(text)
	}
	lr, err := trainSquaredHinge(x, labels, model.BalancedWeights(labels), epochs, l2)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(m.Weights, lr.Weights) || m.Bias != lr.Bias {
		t.Fatal("candidate objective differs from OOF trainer")
	}
	path := filepath.Join(t.TempDir(), "candidate.json")
	if err := model.SaveNew(path, m); err != nil {
		t.Fatal(err)
	}
	loaded, err := model.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"ordinary meeting benign", "incident malware threat", "unknown"} {
		before, after := classify.New(m).Classify(text), classify.New(loaded).Classify(text)
		if !reflect.DeepEqual(before, after) {
			t.Fatal("serialized hinge runtime changed")
		}
	}
	if loaded.Validation != nil {
		t.Fatal("candidate invented validation evidence")
	}
	if _, err := fitFullTrainCandidateWithLoss(groups, 2, "provisional-invalid", "unknown"); err == nil {
		t.Fatal("unknown objective accepted")
	}
}
func TestCandidateRejectsDegenerateTraining(t *testing.T) {
	for _, groups := range [][]group{nil, {{text: "common", label: 1}}, {{text: "one", label: 0}, {text: "two", label: 1}}, {{text: "common", label: 2}, {text: "common", label: 0}}} {
		if _, err := fitFullTrainCandidate(groups, 2, "provisional-test"); err == nil {
			t.Fatalf("accepted %v", groups)
		}
	}
}
