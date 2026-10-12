# Classifieur de pertinence CTI — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build `apps/classifier`, a standalone Go service that scores each scraped article in MongoDB as "pertinent CTI" or not (TF-IDF + logistic regression, trained offline), writing a `relevance` field back onto the `article` document — upstream of the (not-yet-merged) entity parser.

**Architecture:** Two offline CLI tools (`cmd/bootstrap`, `cmd/train`) produce a self-contained `model.json` (vocabulary + IDF + logistic regression weights + threshold). A third binary (`cmd/main`, the long-running service) loads that file once at startup and polls Mongo on a ticker, exactly like `apps/scraper` and `apps/crawler` already do, scoring any `article` document missing a `relevance` field.

**Tech Stack:** Go 1.24, `go.mongodb.org/mongo-driver`, `github.com/joho/godotenv`. No ML library — TF-IDF and logistic regression are implemented directly (small, dependency-free algorithms).

Design reference: `docs/plans/2026-10-01-relevance-classifier-design.md`. Read it before starting if anything below is ambiguous about *why*.

---

## Before you start

This is a **new Go module**, independent from `apps/scraper`'s module (same pattern as the rest of the repo — each `apps/*` has its own `go.mod`). All `go` commands in this plan must be run with working directory `apps/classifier` (the plan notes this per command).

---

### Task 1: Scaffold the Go module and directory layout

**Files:**
- Create: `apps/classifier/go.mod`
- Create: `apps/classifier/.gitignore`

**Step 1: Create the directory and initialize the module**

Run:
```bash
mkdir -p apps/classifier/cmd/main apps/classifier/cmd/bootstrap apps/classifier/cmd/train
mkdir -p apps/classifier/internal/text apps/classifier/internal/tfidf apps/classifier/internal/model apps/classifier/internal/classify apps/classifier/internal/bootstrap apps/classifier/internal/db
mkdir -p apps/classifier/data
cd apps/classifier && go mod init github.com/S-rvo/BlueWhiteThreat/apps/classifier
```

Expected: `apps/classifier/go.mod` created with `module github.com/S-rvo/BlueWhiteThreat/apps/classifier`.

**Step 2: Set the Go version to match the rest of the repo**

Edit `apps/classifier/go.mod`, ensure the top reads:
```
module github.com/S-rvo/BlueWhiteThreat/apps/classifier

go 1.24
```

**Step 3: Add a `.gitignore` for build artifacts and local training data**

```
classifier
*.csv
model.json
.env
```

(`model.json` and the training CSV are build outputs of `cmd/train`/`cmd/bootstrap` — they're regenerated locally, not committed. A real trained `model.json` will later be committed deliberately once the model is good enough to ship, by removing this ignore line — not part of this plan.)

**Step 4: Commit**

```bash
git add apps/classifier/go.mod apps/classifier/.gitignore
git commit -m "chore(classifier): scaffold classifier module skeleton"
```

---

### Task 2: Tokenizer

**Files:**
- Create: `apps/classifier/internal/text/tokenize.go`
- Test: `apps/classifier/internal/text/tokenize_test.go`

**Step 1: Write the failing test**

```go
// apps/classifier/internal/text/tokenize_test.go
package text

import (
	"reflect"
	"testing"
)

func TestTokenize(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "lowercases and strips punctuation",
			in:   "LockBit 3.0 a attaqué l'entreprise!",
			want: []string{"lockbit", "attaqué", "entreprise"},
		},
		{
			name: "drops single-character tokens",
			in:   "a b cd",
			want: []string{"cd"},
		},
		{
			name: "empty string yields no tokens",
			in:   "",
			want: []string{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Tokenize(c.in)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("Tokenize(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run (from `apps/classifier`): `go test ./internal/text/... -v`
Expected: FAIL — `Tokenize` undefined.

**Step 3: Write minimal implementation**

```go
// apps/classifier/internal/text/tokenize.go
package text

import (
	"regexp"
	"strings"
)

var tokenPattern = regexp.MustCompile(`[\p{L}\p{N}]+`)

// Tokenize lowercases s and splits it into unicode-aware word tokens,
// dropping anything shorter than 2 characters (single letters, lone digits)
// since they carry no discriminative signal for TF-IDF.
func Tokenize(s string) []string {
	lower := strings.ToLower(s)
	matches := tokenPattern.FindAllString(lower, -1)
	tokens := make([]string, 0, len(matches))
	for _, m := range matches {
		if len([]rune(m)) < 2 {
			continue
		}
		tokens = append(tokens, m)
	}
	return tokens
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/text/... -v`
Expected: PASS (all 3 subtests).

**Step 5: Commit**

```bash
git add apps/classifier/internal/text
git commit -m "feat(classifier): add unicode-aware tokenizer"
```

---

### Task 3: TF-IDF vectorizer

**Files:**
- Create: `apps/classifier/internal/tfidf/vectorizer.go`
- Test: `apps/classifier/internal/tfidf/vectorizer_test.go`
- Modify: `apps/classifier/go.mod` (no new dependency needed — uses only internal/text and stdlib)

**Step 1: Write the failing test**

```go
// apps/classifier/internal/tfidf/vectorizer_test.go
package tfidf

import "testing"

func TestFitAndTransform(t *testing.T) {
	docs := []string{
		"ransomware lockbit encrypted the servers",
		"the quarterly sales report was published",
		"a new ransomware campaign targets hospitals",
	}

	v := Fit(docs)

	if len(v.Vocabulary) == 0 {
		t.Fatal("expected a non-empty vocabulary")
	}
	if len(v.IDF) != len(v.Vocabulary) {
		t.Fatalf("IDF length %d does not match vocabulary size %d", len(v.IDF), len(v.Vocabulary))
	}

	vec := v.Transform("ransomware lockbit encrypted the servers")
	if len(vec) != len(v.Vocabulary) {
		t.Fatalf("vector length %d does not match vocabulary size %d", len(vec), len(v.Vocabulary))
	}

	ransomwareIdx, ok := v.Vocabulary["ransomware"]
	if !ok {
		t.Fatal("expected 'ransomware' to be in the vocabulary")
	}
	if vec[ransomwareIdx] <= 0 {
		t.Fatalf("expected a positive weight for 'ransomware', got %f", vec[ransomwareIdx])
	}

	empty := v.Transform("")
	for i, x := range empty {
		if x != 0 {
			t.Fatalf("expected zero vector for empty document, got non-zero at index %d", i)
		}
	}

	unknownOnly := v.Transform("zzqqxx nonexistentword")
	for i, x := range unknownOnly {
		if x != 0 {
			t.Fatalf("expected zero vector for out-of-vocabulary document, got non-zero at index %d", i)
		}
	}
}
```

**Step 2: Run test to verify it fails**

From `apps/classifier`: `go test ./internal/tfidf/... -v`
Expected: FAIL — package `tfidf` / `Fit` undefined.

**Step 3: Write minimal implementation**

```go
// apps/classifier/internal/tfidf/vectorizer.go
package tfidf

import (
	"math"
	"sort"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/text"
)

// MaxVocabularySize bounds the model's memory/file size and keeps training
// fast; terms beyond this cutoff are the least-common ones, which contribute
// the least discriminative signal anyway.
const MaxVocabularySize = 5000

// Vectorizer turns raw text into a fixed-length TF-IDF vector, given a
// vocabulary and IDF weights learned once at training time (Fit) and then
// reused unchanged at inference time (Transform).
type Vectorizer struct {
	Vocabulary map[string]int
	IDF        []float64
}

// Fit learns a vocabulary (capped at MaxVocabularySize, most-frequent terms
// first) and per-term IDF weights from a training corpus.
func Fit(documents []string) *Vectorizer {
	documentFrequency := map[string]int{}
	for _, doc := range documents {
		seen := map[string]bool{}
		for _, tok := range text.Tokenize(doc) {
			if !seen[tok] {
				documentFrequency[tok]++
				seen[tok] = true
			}
		}
	}

	type termDF struct {
		term string
		df   int
	}
	terms := make([]termDF, 0, len(documentFrequency))
	for t, c := range documentFrequency {
		terms = append(terms, termDF{t, c})
	}
	sort.Slice(terms, func(i, j int) bool {
		if terms[i].df != terms[j].df {
			return terms[i].df > terms[j].df
		}
		return terms[i].term < terms[j].term // stable tie-break
	})
	if len(terms) > MaxVocabularySize {
		terms = terms[:MaxVocabularySize]
	}

	n := len(documents)
	vocabulary := make(map[string]int, len(terms))
	idf := make([]float64, len(terms))
	for i, t := range terms {
		vocabulary[t.term] = i
		idf[i] = math.Log(float64(n+1)/float64(t.df+1)) + 1
	}

	return &Vectorizer{Vocabulary: vocabulary, IDF: idf}
}

// Transform converts a document into a TF-IDF vector over the fitted
// vocabulary. Tokens outside the vocabulary are silently ignored.
func (v *Vectorizer) Transform(document string) []float64 {
	vector := make([]float64, len(v.Vocabulary))
	tokens := text.Tokenize(document)
	if len(tokens) == 0 {
		return vector
	}

	termFrequency := map[int]int{}
	for _, tok := range tokens {
		if idx, ok := v.Vocabulary[tok]; ok {
			termFrequency[idx]++
		}
	}

	total := float64(len(tokens))
	for idx, count := range termFrequency {
		vector[idx] = (float64(count) / total) * v.IDF[idx]
	}
	return vector
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tfidf/... -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add apps/classifier/internal/tfidf
git commit -m "feat(classifier): add TF-IDF vectorizer"
```

---

### Task 4: Logistic regression (train + predict)

**Files:**
- Create: `apps/classifier/internal/model/logreg.go`
- Test: `apps/classifier/internal/model/logreg_test.go`

**Step 1: Write the failing test**

```go
// apps/classifier/internal/model/logreg_test.go
package model

import "testing"

func TestTrainConvergesOnSeparableData(t *testing.T) {
	// 1D feature, clearly separable: label 0 for small x, label 1 for large x.
	X := [][]float64{{0}, {0}, {1}, {4}, {5}, {5}}
	y := []float64{0, 0, 0, 1, 1, 1}

	lr := Train(X, y, 2000, 0.1, 0.0)

	lowScore := lr.Predict([]float64{0})
	highScore := lr.Predict([]float64{5})

	if lowScore >= 0.5 {
		t.Fatalf("expected low score for x=0, got %f", lowScore)
	}
	if highScore < 0.5 {
		t.Fatalf("expected high score for x=5, got %f", highScore)
	}
}
```

**Step 2: Run test to verify it fails**

From `apps/classifier`: `go test ./internal/model/... -v`
Expected: FAIL — `Train`/`Predict` undefined.

**Step 3: Write minimal implementation**

```go
// apps/classifier/internal/model/logreg.go
package model

import "math"

// LogisticRegression is a binary classifier: sigmoid(dot(weights, x) + bias).
type LogisticRegression struct {
	Weights []float64
	Bias    float64
}

func sigmoid(z float64) float64 {
	return 1 / (1 + math.Exp(-z))
}

// Predict returns the probability (0..1) that x belongs to the positive class.
func (m *LogisticRegression) Predict(x []float64) float64 {
	z := m.Bias
	for i, v := range x {
		z += v * m.Weights[i]
	}
	return sigmoid(z)
}

// Train fits weights and bias via batch gradient descent on binary cross-entropy
// loss, with L2 regularization on the weights (not the bias).
func Train(X [][]float64, y []float64, epochs int, learningRate, l2 float64) *LogisticRegression {
	nFeatures := len(X[0])
	weights := make([]float64, nFeatures)
	bias := 0.0
	n := float64(len(X))

	for epoch := 0; epoch < epochs; epoch++ {
		gradWeights := make([]float64, nFeatures)
		gradBias := 0.0

		for i, x := range X {
			prediction := sigmoid(dot(weights, x) + bias)
			errorTerm := prediction - y[i]
			for j, v := range x {
				gradWeights[j] += errorTerm * v
			}
			gradBias += errorTerm
		}

		for j := range weights {
			weights[j] -= learningRate * (gradWeights[j]/n + l2*weights[j])
		}
		bias -= learningRate * (gradBias / n)
	}

	return &LogisticRegression{Weights: weights, Bias: bias}
}

func dot(a, b []float64) float64 {
	sum := 0.0
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/model/... -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add apps/classifier/internal/model/logreg.go apps/classifier/internal/model/logreg_test.go
git commit -m "feat(classifier): add logistic regression train/predict"
```

---

### Task 5: Model persistence (save/load)

**Files:**
- Create: `apps/classifier/internal/model/persist.go`
- Test: `apps/classifier/internal/model/persist_test.go`

**Step 1: Write the failing test**

```go
// apps/classifier/internal/model/persist_test.go
package model

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSaveAndLoadRoundTrip(t *testing.T) {
	original := &Model{
		Vocabulary: map[string]int{"ransomware": 0, "leak": 1},
		IDF:        []float64{1.2, 0.8},
		Weights:    []float64{0.5, -0.3},
		Bias:       0.1,
		Threshold:  0.5,
		Version:    "test-v1",
	}

	path := filepath.Join(t.TempDir(), "model.json")

	if err := Save(path, original); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if !reflect.DeepEqual(original, loaded) {
		t.Fatalf("round-tripped model differs: got %+v, want %+v", loaded, original)
	}
}

func TestLoadMissingFileFails(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err == nil {
		t.Fatal("expected an error loading a missing model file, got nil")
	}
}

var _ = os.Getenv // keep os imported if unused by future edits
```

(Drop the trailing `var _ = os.Getenv` line if you don't end up needing `os` directly — written here only to avoid an unused-import error if you copy this verbatim and trim imports later. Simplest: just don't import `os` at all since `t.TempDir()` and `filepath` are enough.)

**Step 2: Run test to verify it fails**

From `apps/classifier`: `go test ./internal/model/... -v`
Expected: FAIL — `Model`/`Save`/`Load` undefined.

**Step 3: Write minimal implementation**

```go
// apps/classifier/internal/model/persist.go
package model

import (
	"encoding/json"
	"fmt"
	"os"
)

// Model is the fully self-contained artifact produced by cmd/train and
// loaded by the classifier service at startup: a fitted TF-IDF vocabulary
// paired with the logistic regression weights trained on top of it.
type Model struct {
	Vocabulary map[string]int `json:"vocabulary"`
	IDF        []float64      `json:"idf"`
	Weights    []float64      `json:"weights"`
	Bias       float64        `json:"bias"`
	Threshold  float64        `json:"threshold"`
	Version    string         `json:"version"`
}

func Save(path string, m *Model) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal model: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write model file: %w", err)
	}
	return nil
}

func Load(path string) (*Model, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read model file: %w", err)
	}
	var m Model
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("unmarshal model: %w", err)
	}
	return &m, nil
}
```

Remove the `var _ = os.Getenv` placeholder and the unused `os` import from the test file before running — the test only needs `path/filepath`, `reflect`, and `testing`.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/model/... -v`
Expected: PASS (both tests).

**Step 5: Commit**

```bash
git add apps/classifier/internal/model/persist.go apps/classifier/internal/model/persist_test.go
git commit -m "feat(classifier): add model.json save/load"
```

---

### Task 6: Seed keywords + bootstrap labeling

**Files:**
- Create: `apps/classifier/data/seed_keywords.json`
- Create: `apps/classifier/internal/bootstrap/label.go`
- Test: `apps/classifier/internal/bootstrap/label_test.go`

**Step 1: Create the seed keyword list**

```json
[
  "ransomware",
  "malware",
  "exploit",
  "vulnerability",
  "cve",
  "breach",
  "data leak",
  "leaked",
  "phishing",
  "backdoor",
  "botnet",
  "ddos",
  "c2",
  "command and control",
  "zero-day",
  "trojan",
  "spyware",
  "credential",
  "compromise",
  "threat actor",
  "apt",
  "darkweb",
  "dark web",
  "hacker",
  "hacked",
  "ransom",
  "encryption key",
  "stolen data"
]
```

**Step 2: Write the failing test**

```go
// apps/classifier/internal/bootstrap/label_test.go
package bootstrap

import "testing"

func TestLabelByKeywords(t *testing.T) {
	keywords := []string{"ransomware", "leaked", "exploit"}

	cases := []struct {
		name string
		text string
		want int
	}{
		{
			name: "two keyword hits labels relevant",
			text: "A new ransomware strain has leaked victim data online.",
			want: 1,
		},
		{
			name: "single keyword hit labels not relevant",
			text: "Researchers found a minor ransomware mention in passing.",
			want: 0,
		},
		{
			name: "no keyword hits labels not relevant",
			text: "The quarterly earnings report exceeded expectations.",
			want: 0,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := LabelByKeywords(c.text, keywords)
			if got != c.want {
				t.Fatalf("LabelByKeywords(%q) = %d, want %d", c.text, got, c.want)
			}
		})
	}
}

func TestLoadKeywords(t *testing.T) {
	keywords, err := LoadKeywords("../../data/seed_keywords.json")
	if err != nil {
		t.Fatalf("LoadKeywords failed: %v", err)
	}
	if len(keywords) == 0 {
		t.Fatal("expected a non-empty keyword list")
	}
}
```

**Step 3: Run test to verify it fails**

From `apps/classifier`: `go test ./internal/bootstrap/... -v`
Expected: FAIL — `LabelByKeywords`/`LoadKeywords` undefined.

**Step 4: Write minimal implementation**

```go
// apps/classifier/internal/bootstrap/label.go
package bootstrap

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// MinKeywordHits is the number of distinct seed keywords that must appear
// in a document for the bootstrap pass to provisionally label it relevant.
// This is a cheap, noisy heuristic meant to be corrected by a human before
// training — see docs/plans/2026-10-01-relevance-classifier-design.md.
const MinKeywordHits = 2

// LabelByKeywords returns 1 (relevant) if text contains at least
// MinKeywordHits of the given keywords, 0 otherwise.
func LabelByKeywords(text string, keywords []string) int {
	lower := strings.ToLower(text)
	hits := 0
	for _, kw := range keywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			hits++
		}
	}
	if hits >= MinKeywordHits {
		return 1
	}
	return 0
}

// LoadKeywords reads the seed keyword list from a JSON file (array of strings).
func LoadKeywords(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read keywords file: %w", err)
	}
	var keywords []string
	if err := json.Unmarshal(data, &keywords); err != nil {
		return nil, fmt.Errorf("unmarshal keywords: %w", err)
	}
	return keywords, nil
}
```

**Step 5: Run test to verify it passes**

Run: `go test ./internal/bootstrap/... -v`
Expected: PASS.

**Step 6: Commit**

```bash
git add apps/classifier/data/seed_keywords.json apps/classifier/internal/bootstrap
git commit -m "feat(classifier): add seed keyword list and bootstrap labeling"
```

---

### Task 7: Classification orchestrator

**Files:**
- Create: `apps/classifier/internal/classify/classify.go`
- Test: `apps/classifier/internal/classify/classify_test.go`

This wraps a loaded `model.Model` into a ready-to-use classifier, combining the vectorizer and the logistic regression into one `Classify(text) Result` call — the shape the service loop (Task 10) and the training evaluation step (Task 9) both need.

**Step 1: Write the failing test**

```go
// apps/classifier/internal/classify/classify_test.go
package classify

import (
	"testing"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/model"
)

func TestClassify(t *testing.T) {
	// Hand-built tiny model: vocabulary of one term, positive weight,
	// so any document containing "ransomware" scores above threshold
	// and any document without it scores at the bias (below threshold).
	m := &model.Model{
		Vocabulary: map[string]int{"ransomware": 0},
		IDF:        []float64{1.0},
		Weights:    []float64{5.0},
		Bias:       -1.0,
		Threshold:  0.5,
		Version:    "unit-test",
	}

	c := New(m)

	relevant := c.Classify("a new ransomware campaign is spreading")
	if !relevant.Relevant {
		t.Fatalf("expected relevant=true, got score=%f", relevant.Score)
	}
	if relevant.ModelVersion != "unit-test" {
		t.Fatalf("expected model version to be propagated, got %q", relevant.ModelVersion)
	}

	notRelevant := c.Classify("quarterly earnings exceeded expectations")
	if notRelevant.Relevant {
		t.Fatalf("expected relevant=false, got score=%f", notRelevant.Score)
	}
}
```

**Step 2: Run test to verify it fails**

From `apps/classifier`: `go test ./internal/classify/... -v`
Expected: FAIL — package `classify` / `New` undefined.

**Step 3: Write minimal implementation**

```go
// apps/classifier/internal/classify/classify.go
package classify

import (
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/model"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/tfidf"
)

// Result is the outcome of scoring one document.
type Result struct {
	Relevant     bool
	Score        float64
	ModelVersion string
}

// Classifier combines a fitted vectorizer and a trained logistic regression
// into a single Classify(text) call, so callers never need to know about
// TF-IDF or model internals.
type Classifier struct {
	vectorizer *tfidf.Vectorizer
	logreg     *model.LogisticRegression
	threshold  float64
	version    string
}

// New builds a ready-to-use Classifier from a loaded model.Model.
func New(m *model.Model) *Classifier {
	return &Classifier{
		vectorizer: &tfidf.Vectorizer{Vocabulary: m.Vocabulary, IDF: m.IDF},
		logreg:     &model.LogisticRegression{Weights: m.Weights, Bias: m.Bias},
		threshold:  m.Threshold,
		version:    m.Version,
	}
}

func (c *Classifier) Classify(text string) Result {
	vector := c.vectorizer.Transform(text)
	score := c.logreg.Predict(vector)
	return Result{
		Relevant:     score >= c.threshold,
		Score:        score,
		ModelVersion: c.version,
	}
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/classify/... -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add apps/classifier/internal/classify
git commit -m "feat(classifier): add classification orchestrator"
```

---

### Task 8: MongoDB connector

**Files:**
- Create: `apps/classifier/internal/db/mongo.go`
- Modify: `apps/classifier/go.mod` (adds `go.mongodb.org/mongo-driver` and `github.com/joho/godotenv`)

No unit test here — this package only wraps the Mongo driver (network I/O), consistent with `apps/scraper/internal/db/mongo.go` which is also untested directly and instead exercised through the services that use it. You'll verify it manually in Task 11's end-to-end check.

**Step 1: Add dependencies**

Run (from `apps/classifier`):
```bash
go get go.mongodb.org/mongo-driver@v1.17.4
go get github.com/joho/godotenv@v1.5.1
```

**Step 2: Write the connector**

```go
// apps/classifier/internal/db/mongo.go
package db

import (
	"context"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func getEnvOrDefault(key, defaultValue string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultValue
}

// Connect opens a MongoDB connection using the same MONGO_* environment
// variables as apps/scraper, so both services can share one .env in compose.
func Connect(ctx context.Context) (*mongo.Client, error) {
	user := getEnvOrDefault("MONGO_INITDB_ROOT_USERNAME", "root")
	pass := getEnvOrDefault("MONGO_INITDB_ROOT_PASSWORD", "root")
	host := getEnvOrDefault("MONGO_HOST", "localhost")
	port := getEnvOrDefault("MONGO_PORT", "27017")

	uri := "mongodb://" + user + ":" + pass + "@" + host + ":" + port + "/"
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return mongo.Connect(ctx, options.Client().ApplyURI(uri))
}

// ArticleContent is the minimal projection needed for scoring: the article's
// Mongo _id and its scraped text.
type ArticleContent struct {
	ID      interface{} `bson:"_id"`
	Content string      `bson:"content"`
}

func articleCollection(client *mongo.Client) *mongo.Collection {
	dbName := getEnvOrDefault("MONGO_DB", "BlueWhiteThreat")
	return client.Database(dbName).Collection("article")
}

// FetchAllContent returns every article's id and content, regardless of
// relevance status — used by cmd/bootstrap to build a labeling candidate set.
func FetchAllContent(ctx context.Context, client *mongo.Client) ([]ArticleContent, error) {
	cursor, err := articleCollection(client).Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []ArticleContent
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

// FetchUnclassified returns articles that have not yet been scored, i.e. that
// have no "relevance" field at all — distinct from articles scored as
// not-relevant, which must not be re-processed on every tick.
func FetchUnclassified(ctx context.Context, client *mongo.Client) ([]ArticleContent, error) {
	filter := bson.M{"relevance": bson.M{"$exists": false}}
	cursor, err := articleCollection(client).Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []ArticleContent
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

// Relevance mirrors the "relevance" subdocument written onto each article.
type Relevance struct {
	Relevant     bool    `bson:"relevant"`
	Score        float64 `bson:"score"`
	ModelVersion string  `bson:"model_version"`
}

// UpdateRelevance sets the "relevance" field on the article identified by id.
func UpdateRelevance(ctx context.Context, client *mongo.Client, id interface{}, r Relevance) error {
	_, err := articleCollection(client).UpdateOne(
		ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{"relevance": r}},
	)
	return err
}
```

**Step 3: Verify it builds**

Run: `go build ./...`
Expected: no errors.

**Step 4: Commit**

```bash
git add apps/classifier/go.mod apps/classifier/go.sum apps/classifier/internal/db
git commit -m "feat(classifier): add MongoDB connector for article read/update"
```

---

### Task 9: `cmd/bootstrap` — generate labeling candidates from Mongo

**Files:**
- Create: `apps/classifier/cmd/bootstrap/main.go`

**Step 1: Write the command**

```go
// apps/classifier/cmd/bootstrap/main.go
package main

import (
	"context"
	"encoding/csv"
	"log"
	"os"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/bootstrap"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/db"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env found or unreadable, continuing with existing environment.")
	}

	outputPath := getEnvOrDefault("BOOTSTRAP_OUTPUT", "training_data.csv")
	keywordsPath := getEnvOrDefault("SEED_KEYWORDS_PATH", "data/seed_keywords.json")

	keywords, err := bootstrap.LoadKeywords(keywordsPath)
	if err != nil {
		log.Fatalf("failed to load seed keywords: %v", err)
	}

	ctx := context.Background()
	client, err := db.Connect(ctx)
	if err != nil {
		log.Fatalf("failed to connect to MongoDB: %v", err)
	}
	defer client.Disconnect(ctx)

	articles, err := db.FetchAllContent(ctx, client)
	if err != nil {
		log.Fatalf("failed to fetch articles: %v", err)
	}
	log.Printf("Fetched %d articles from MongoDB", len(articles))

	file, err := os.Create(outputPath)
	if err != nil {
		log.Fatalf("failed to create output file: %v", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	if err := writer.Write([]string{"text", "label"}); err != nil {
		log.Fatalf("failed to write CSV header: %v", err)
	}

	written := 0
	for _, article := range articles {
		if article.Content == "" {
			continue
		}
		label := bootstrap.LabelByKeywords(article.Content, keywords)
		if err := writer.Write([]string{article.Content, itoa(label)}); err != nil {
			log.Printf("skipping article due to write error: %v", err)
			continue
		}
		written++
	}

	log.Printf("Wrote %d labeled candidates to %s — review and correct labels by hand before running cmd/train.", written, outputPath)
}

func getEnvOrDefault(key, defaultValue string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultValue
}

func itoa(i int) string {
	if i == 1 {
		return "1"
	}
	return "0"
}
```

**Step 2: Verify it builds**

Run (from `apps/classifier`): `go build ./cmd/bootstrap/...`
Expected: no errors.

**Step 3: Commit**

```bash
git add apps/classifier/cmd/bootstrap
git commit -m "feat(classifier): add bootstrap CLI to pre-label articles from Mongo"
```

(Manual run against a real Mongo instance — with the stack up via `docker-compose` — and manual correction of the resulting CSV happens later, outside this plan, once there's a database to bootstrap from.)

---

### Task 10: `cmd/train` — fit vectorizer + logistic regression from labeled CSV

**Files:**
- Create: `apps/classifier/cmd/train/main.go`

**Step 1: Write the command**

```go
// apps/classifier/cmd/train/main.go
package main

import (
	"encoding/csv"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/classify"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/model"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/tfidf"
)

const (
	trainSplitRatio = 0.8
	epochs          = 500
	learningRate    = 0.1
	l2              = 0.001
	threshold       = 0.5
)

func main() {
	inputPath := getEnvOrDefault("TRAINING_DATA", "training_data.csv")
	outputPath := getEnvOrDefault("MODEL_PATH", "model.json")

	texts, labels, err := readLabeledCSV(inputPath)
	if err != nil {
		log.Fatalf("failed to read training data: %v", err)
	}
	if len(texts) < 10 {
		log.Fatalf("need at least 10 labeled examples, got %d — label more data first", len(texts))
	}

	trainTexts, trainLabels, evalTexts, evalLabels := split(texts, labels, trainSplitRatio)
	log.Printf("Training on %d examples, evaluating on %d", len(trainTexts), len(evalTexts))

	vectorizer := tfidf.Fit(trainTexts)

	X := make([][]float64, len(trainTexts))
	for i, t := range trainTexts {
		X[i] = vectorizer.Transform(t)
	}

	lr := model.Train(X, trainLabels, epochs, learningRate, l2)

	m := &model.Model{
		Vocabulary: vectorizer.Vocabulary,
		IDF:        vectorizer.IDF,
		Weights:    lr.Weights,
		Bias:       lr.Bias,
		Threshold:  threshold,
		Version:    time.Now().Format("2006-01-02T15-04-05"),
	}

	evaluate(m, evalTexts, evalLabels)

	if err := model.Save(outputPath, m); err != nil {
		log.Fatalf("failed to save model: %v", err)
	}
	log.Printf("Saved model to %s (version %s)", outputPath, m.Version)
}

func readLabeledCSV(path string) (texts []string, labels []float64, err error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("parse csv: %w", err)
	}
	if len(rows) < 2 {
		return nil, nil, fmt.Errorf("expected a header row plus at least one data row")
	}

	for _, row := range rows[1:] { // skip header
		if len(row) != 2 {
			continue
		}
		label, err := strconv.ParseFloat(row[1], 64)
		if err != nil {
			continue
		}
		texts = append(texts, row[0])
		labels = append(labels, label)
	}
	return texts, labels, nil
}

func split(texts []string, labels []float64, trainRatio float64) (trainTexts []string, trainLabels []float64, evalTexts []string, evalLabels []float64) {
	n := len(texts)
	indices := rand.Perm(n)
	cut := int(float64(n) * trainRatio)

	for i, idx := range indices {
		if i < cut {
			trainTexts = append(trainTexts, texts[idx])
			trainLabels = append(trainLabels, labels[idx])
		} else {
			evalTexts = append(evalTexts, texts[idx])
			evalLabels = append(evalLabels, labels[idx])
		}
	}
	return
}

func evaluate(m *model.Model, texts []string, labels []float64) {
	c := classify.New(m)

	var truePositive, falsePositive, trueNegative, falseNegative int
	for i, text := range texts {
		result := c.Classify(text)
		predicted := 0
		if result.Relevant {
			predicted = 1
		}
		actual := int(labels[i])

		switch {
		case predicted == 1 && actual == 1:
			truePositive++
		case predicted == 1 && actual == 0:
			falsePositive++
		case predicted == 0 && actual == 0:
			trueNegative++
		case predicted == 0 && actual == 1:
			falseNegative++
		}
	}

	total := len(texts)
	accuracy := float64(truePositive+trueNegative) / float64(total)
	precision := safeDiv(float64(truePositive), float64(truePositive+falsePositive))
	recall := safeDiv(float64(truePositive), float64(truePositive+falseNegative))
	f1 := safeDiv(2*precision*recall, precision+recall)

	log.Printf("Eval metrics — accuracy: %.3f, precision: %.3f, recall: %.3f, f1: %.3f (n=%d)",
		accuracy, precision, recall, f1, total)
}

func safeDiv(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}

func getEnvOrDefault(key, defaultValue string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultValue
}
```

**Step 2: Verify it builds**

Run (from `apps/classifier`): `go build ./cmd/train/...`
Expected: no errors.

**Step 3: Commit**

```bash
git add apps/classifier/cmd/train
git commit -m "feat(classifier): add train CLI to fit model.json from labeled CSV"
```

(Manual run against a real labeled CSV — produced and corrected via Task 9's bootstrap — happens later, once there's enough labeled data; this is noted in the design doc as a deliberately manual step.)

---

### Task 11: `cmd/main` — the long-running classifier service

**Files:**
- Create: `apps/classifier/cmd/main/main.go`

**Step 1: Write the service**

```go
// apps/classifier/cmd/main/main.go
package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/classify"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/db"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/model"
	"github.com/joho/godotenv"
)

var defaultIntervalMinutes = 10

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env found or unreadable, continuing with existing environment.")
	}

	if i, err := strconv.Atoi(os.Getenv("CLASSIFIER_INTERVAL_MIN")); err == nil && i > 0 {
		defaultIntervalMinutes = i
	}

	modelPath := getEnvOrDefault("MODEL_PATH", "model.json")
	m, err := model.Load(modelPath)
	if err != nil {
		log.Fatalf("failed to load model from %s: %v (run cmd/train first)", modelPath, err)
	}

	if override := os.Getenv("RELEVANCE_THRESHOLD"); override != "" {
		if v, err := strconv.ParseFloat(override, 64); err == nil {
			m.Threshold = v
		}
	}

	classifier := classify.New(m)

	log.Printf("Classifier service started (model version %s, interval %d min)", m.Version, defaultIntervalMinutes)
	runOnce(classifier)

	ticker := time.NewTicker(time.Duration(defaultIntervalMinutes) * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		log.Printf("Scheduled classification run at %s", time.Now().Format("15:04:05"))
		runOnce(classifier)
	}
}

func runOnce(classifier *classify.Classifier) {
	ctx := context.Background()
	client, err := db.Connect(ctx)
	if err != nil {
		log.Printf("failed to connect to MongoDB, will retry next tick: %v", err)
		return
	}
	defer client.Disconnect(ctx)

	articles, err := db.FetchUnclassified(ctx, client)
	if err != nil {
		log.Printf("failed to fetch unclassified articles, will retry next tick: %v", err)
		return
	}
	log.Printf("Found %d unclassified articles", len(articles))

	for _, article := range articles {
		if article.Content == "" {
			log.Printf("skipping article %v: empty content", article.ID)
			continue
		}

		result := classifier.Classify(article.Content)
		relevance := db.Relevance{
			Relevant:     result.Relevant,
			Score:        result.Score,
			ModelVersion: result.ModelVersion,
		}

		if err := db.UpdateRelevance(ctx, client, article.ID, relevance); err != nil {
			log.Printf("failed to update relevance for article %v: %v", article.ID, err)
			continue
		}
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultValue
}
```

**Step 2: Verify it builds**

Run (from `apps/classifier`): `go build ./cmd/main/...`
Expected: no errors.

**Step 3: Run the full test suite for the module**

Run (from `apps/classifier`): `go test ./...`
Expected: PASS across all packages (`text`, `tfidf`, `model`, `bootstrap`, `classify`).

**Step 4: Commit**

```bash
git add apps/classifier/cmd/main
git commit -m "feat(classifier): add classifier service with Mongo polling loop"
```

---

### Task 12: Dockerfile and env template

**Files:**
- Create: `apps/classifier/Dockerfile.dev`
- Create: `apps/classifier/.env.example`

**Step 1: Write the Dockerfile**

```dockerfile
# apps/classifier/Dockerfile.dev
FROM golang:1.24

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Run unit tests. Fails the build if any test is red.
RUN go test ./...

# Build the binary for a basic sanity check.
RUN go build -o classifier ./cmd/main

CMD ["go", "run", "cmd/main/main.go"]
```

**Step 2: Write the env template**

```
# apps/classifier/.env.example
MONGO_HOST=
MONGO_PORT=
MONGO_INITDB_ROOT_USERNAME=
MONGO_INITDB_ROOT_PASSWORD=
MONGO_DB=

CLASSIFIER_INTERVAL_MIN=
MODEL_PATH=
RELEVANCE_THRESHOLD=
```

**Step 3: Commit**

```bash
git add apps/classifier/Dockerfile.dev apps/classifier/.env.example
git commit -m "chore(classifier): add Dockerfile.dev and env template"
```

---

### Task 13: Wire into docker-compose

**Files:**
- Modify: `docker-compose.yaml`

**Step 1: Add the `classifier` service**

In `docker-compose.yaml`, add a new service alongside `scraper` (same `volumes`/`depends_on`/`env_file` shape):

```yaml
  classifier:
    build:
      context: ./apps/classifier
      dockerfile: Dockerfile.dev
    container_name: classifier_service
    volumes:
      - ./apps/classifier:/app
    depends_on:
      - mongodb
    env_file:
      - ./apps/classifier/.env
```

**Step 2: Verify the compose file is syntactically valid**

Run: `docker compose config > /dev/null`
Expected: no errors (this will fail if `apps/classifier/.env` doesn't exist yet — copy `.env.example` to `.env` locally first; `.env` itself stays untracked, same as `apps/scraper/.env`).

**Step 3: Commit**

```bash
git add docker-compose.yaml
git commit -m "feat(classifier): wire classifier service into docker-compose"
```

---

### Task 14: End-to-end manual verification

Not a code change — this is the checklist to actually prove the pipeline works, since none of the previous unit tests touch a real database or a real trained model.

1. `cp apps/classifier/.env.example apps/classifier/.env` and fill in the same Mongo credentials used by `apps/scraper/.env`.
2. Bring up Mongo and the scraper so there's real article content to work with: `docker compose up -d mongodb scraper` and let it scrape at least a few dozen articles.
3. From `apps/classifier`, run the bootstrap tool against that live Mongo (either `go run ./cmd/bootstrap` locally with matching `MONGO_HOST=localhost`, or build it into the container): confirm `training_data.csv` is created and has a `text,label` row per article.
4. Open `training_data.csv` and manually correct any obviously wrong labels (the keyword heuristic is a first guess, not ground truth — see design doc section "Entraînement").
5. Run `go run ./cmd/train` (or the built binary) against the corrected CSV: confirm it logs accuracy/precision/recall/F1 and writes `model.json`.
6. Run the service (`go run ./cmd/main`, or `docker compose up classifier` once `model.json` is in place) and confirm in the logs that it finds unclassified articles and processes them.
7. Inspect Mongo (via `mongo-express` on `:8082`, already in compose) and confirm `article` documents now carry a `relevance: {relevant, score, model_version}` field.
8. Re-run the service and confirm already-classified articles are *not* reprocessed (log should show 0 or only newly-scraped articles as "unclassified").

If any step fails, fix the underlying code — do not mark this task done until all 8 checks pass against the real stack.
