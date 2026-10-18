package confirmation

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/classify"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/evalmetrics"
)

type Selection struct {
	PoolSHA256 string        `json:"pool_sha256"`
	Seed       int64         `json:"seed"`
	Requested  int           `json:"requested"`
	Excluded   int           `json:"excluded"`
	Documents  []fingerprint `json:"documents"`
}

// Prepare draws without using model scores or labels. The selection is sealed
// before annotation; confirmation rejects dropped/replaced rows. The caller
// must supply an independently collected, eligible pool, not a review queue
// selected by model uncertainty. Lexical filtering changes the eligible pool.
func Prepare(dir, poolPath, root string, n int, seed int64) (*Selection, error) {
	if n <= 0 {
		return nil, fmt.Errorf("sample size must be positive")
	}
	c, err := Load(dir, root)
	if err != nil {
		return nil, err
	}
	pool, err := ReadDocuments(poolPath, false)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(poolPath)
	if err != nil {
		return nil, err
	}
	sort.Slice(pool, func(i, j int) bool { return pool[i].ID < pool[j].ID })
	rand.New(rand.NewSource(seed)).Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	prior := prints(c.Corpus)
	selection := Selection{PoolSHA256: digest(data), Seed: seed, Requested: n}
	var b strings.Builder
	w := csv.NewWriter(&b)
	_ = w.Write([]string{"id", "text", "label"})
	for _, d := range pool {
		p := printOf(d)
		if collision(p, prior) != "" {
			selection.Excluded++
			continue
		}
		selection.Documents = append(selection.Documents, p)
		prior = append(prior, p)
		_ = w.Write([]string{d.ID, d.Text, ""})
		if len(selection.Documents) == n {
			break
		}
	}
	if len(selection.Documents) == 0 {
		return nil, fmt.Errorf("no eligible unseen documents in pool")
	}
	w.Flush()
	if err = w.Error(); err != nil {
		return nil, err
	}
	// Exclusive reservation prevents generating a second, cherry-picked sample.
	if err = writeJSON(filepath.Join(dir, "selection.json"), selection); err != nil {
		return nil, err
	}
	if err = writeExclusive(filepath.Join(dir, "annotations.csv"), []byte(b.String())); err != nil {
		return nil, err
	}
	return &selection, nil
}

type Interval struct {
	Low  float64 `json:"low"`
	High float64 `json:"high"`
}

// Wilson95 returns nil when the denominator is zero (undefined precision).
func Wilson95(success, total int) *Interval {
	if total == 0 {
		return nil
	}
	const z = 1.959963984540054
	n := float64(total)
	p := float64(success) / n
	den := 1 + z*z/n
	center := (p + z*z/(2*n)) / den
	half := z * math.Sqrt(p*(1-p)/n+z*z/(4*n*n)) / den
	return &Interval{math.Max(0, center-half), math.Min(1, center+half)}
}

type Result struct {
	Timestamp       string    `json:"timestamp"`
	ModelSHA256     string    `json:"model_sha256"`
	CorpusSHA256    string    `json:"corpus_sha256"`
	RuntimeSHA256   string    `json:"runtime_sha256"`
	BatchSHA256     string    `json:"batch_sha256"`
	Threshold       float64   `json:"threshold"`
	N               int       `json:"n"`
	TP              int       `json:"tp"`
	FP              int       `json:"fp"`
	TN              int       `json:"tn"`
	FN              int       `json:"fn"`
	Accuracy        float64   `json:"accuracy"`
	Precision       float64   `json:"precision"`
	Recall          float64   `json:"recall"`
	F1              float64   `json:"f1"`
	Precision95     *Interval `json:"precision_95_wilson"`
	Recall95        *Interval `json:"recall_95_wilson"`
	TargetPrecision float64   `json:"target_precision"`
	TargetSupported bool      `json:"target_supported_by_precision_lower_bound"`
}

// Confirm consumes the entire campaign before scoring. Even a crash after
// reservation requires manual audit; it must not silently make the lot fresh
// again. Files are exclusive/append-only by convention, not tamper-proof.
func Confirm(dir, labelsPath, root string) (*Result, error) {
	c, err := Load(dir, root)
	if err != nil {
		return nil, err
	}
	docs, err := ReadDocuments(labelsPath, true)
	if err != nil {
		return nil, err
	}
	if err = checkBatch(docs, c.Corpus); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "selection.json"))
	if err != nil {
		return nil, fmt.Errorf("prepare the unlabeled batch before annotation: %w", err)
	}
	var selection Selection
	if err = json.Unmarshal(data, &selection); err != nil {
		return nil, err
	}
	selected := map[string]string{}
	for _, p := range selection.Documents {
		selected[p.ID] = p.Exact
	}
	if len(docs) != len(selected) {
		return nil, fmt.Errorf("annotation batch must contain all and only the selected documents")
	}
	for _, d := range docs {
		if selected[d.ID] != printOf(d).Exact {
			return nil, fmt.Errorf("document %s was not selected or its text changed", d.ID)
		}
	}
	result := Result{Timestamp: time.Now().UTC().Format(time.RFC3339), ModelSHA256: c.Manifest.ModelSHA256, CorpusSHA256: c.Manifest.CorpusSHA256, RuntimeSHA256: c.Manifest.RuntimeSHA256, BatchSHA256: batchDigest(docs), Threshold: c.Model.Threshold, N: len(docs), TargetPrecision: c.Manifest.TargetPrecision}
	// O_EXCL makes simultaneous and repeated confirmation fail before scoring,
	// including reordered input, relabeling or replacement with a different model.
	if err = reserve(dir, docs, result); err != nil {
		return nil, fmt.Errorf("cannot reserve confirmation (campaign may already be consumed): %w", err)
	}
	scores := make([]float64, len(docs))
	labels := make([]float64, len(docs))
	classifier := classify.New(c.Model)
	for i, d := range docs {
		scores[i] = classifier.Classify(d.Text).Score
		labels[i] = d.Label
	}
	result.TP, result.FP, result.TN, result.FN = evalmetrics.ConfusionCounts(scores, labels, c.Model.Threshold)
	result.Accuracy, result.Precision, result.Recall, result.F1 = evalmetrics.ConfusionAt(scores, labels, c.Model.Threshold)
	result.Precision95 = Wilson95(result.TP, result.TP+result.FP)
	result.Recall95 = Wilson95(result.TP, result.TP+result.FN)
	result.TargetSupported = result.Precision95 != nil && result.Precision95.Low >= result.TargetPrecision
	if err = writeJSON(filepath.Join(dir, "result.json"), result); err != nil {
		return nil, err
	}
	return &result, nil
}
