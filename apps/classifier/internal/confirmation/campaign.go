package confirmation

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/model"
)

type Manifest struct {
	Schema                   int     `json:"schema"`
	CreatedAt                string  `json:"created_at"`
	ModelSHA256              string  `json:"model_sha256"`
	CorpusSHA256             string  `json:"corpus_sha256"`
	RuntimeSHA256            string  `json:"runtime_sha256"`
	ModelVersion             string  `json:"model_version"`
	Threshold                float64 `json:"threshold"`
	ReferenceDocuments       int     `json:"reference_documents"`
	NearDuplicateMaxDistance int     `json:"near_duplicate_max_distance"`
	TargetPrecision          float64 `json:"target_precision"`
}

type Campaign struct {
	Manifest Manifest
	Model    *model.Model
	Corpus   []Document
}

// RuntimeDigest binds inference, leakage guards and preparation code, including
// the scraper text extraction source. Run the CLIs from apps/classifier.
func RuntimeDigest(root string) (string, error) {
	files := []string{"go.mod", "go.sum", "../scraper/internal/sites/content.go", "../scraper/internal/sites/fetch.go"}
	for _, dir := range []string{"internal/text", "internal/tfidf", "internal/model", "internal/classify", "internal/dedup", "internal/confirmation", "internal/evalmetrics", "cmd/confirmfinal", "cmd/prepareconfirmation"} {
		matches, err := filepath.Glob(filepath.Join(root, dir, "*.go"))
		if err != nil {
			return "", err
		}
		if len(matches) == 0 {
			return "", fmt.Errorf("missing runtime sources: %s", dir)
		}
		for _, path := range matches {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return "", err
			}
			files = append(files, rel)
		}
	}
	sort.Strings(files)
	var b strings.Builder
	for _, name := range files {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "%s\x00%s\n", filepath.ToSlash(name), digest(data))
	}
	return digest([]byte(b.String())), nil
}

func validateModel(m *model.Model) error {
	n := len(m.Vocabulary)
	finite := func(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }
	if n == 0 || len(m.IDF) != n || len(m.Weights) != n || !finite(m.Threshold) || m.Threshold < 0 || m.Threshold > 1 || !finite(m.Bias) || !finite(m.Calibration.A) || !finite(m.Calibration.B) {
		return fmt.Errorf("invalid model dimensions or parameters")
	}
	seen := map[int]bool{}
	for _, i := range m.Vocabulary {
		if i < 0 || i >= n || seen[i] || !finite(m.IDF[i]) || !finite(m.Weights[i]) {
			return fmt.Errorf("invalid vocabulary index or model weight")
		}
		seen[i] = true
	}
	return nil
}

func writeExclusive(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
func writeJSON(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return writeExclusive(path, append(data, '\n'))
}

// Freeze never overwrites a campaign. Copying all development text protects
// against future corpus changes making previously used documents look new.
func Freeze(dir, modelPath, corpusPath, root string) (*Campaign, error) {
	modelBytes, err := os.ReadFile(modelPath)
	if err != nil {
		return nil, err
	}
	var m model.Model
	if err = json.Unmarshal(modelBytes, &m); err != nil {
		return nil, err
	}
	if err = validateModel(&m); err != nil {
		return nil, err
	}
	corpusBytes, err := os.ReadFile(corpusPath)
	if err != nil {
		return nil, err
	}
	corpus, err := readDocuments(corpusPath, true, true)
	if err != nil {
		return nil, err
	}
	runtimeHash, err := RuntimeDigest(root)
	if err != nil {
		return nil, err
	}
	manifest := Manifest{Schema: 1, CreatedAt: time.Now().UTC().Format(time.RFC3339), ModelSHA256: digest(modelBytes), CorpusSHA256: digest(corpusBytes), RuntimeSHA256: runtimeHash, ModelVersion: m.Version, Threshold: m.Threshold, ReferenceDocuments: len(corpus), NearDuplicateMaxDistance: 10, TargetPrecision: 0.99}
	if err = os.MkdirAll(filepath.Dir(dir), 0700); err != nil {
		return nil, err
	}
	if err = os.Mkdir(dir, 0700); err != nil {
		return nil, fmt.Errorf("campaign must be new: %w", err)
	}
	for _, f := range []struct {
		name string
		data []byte
	}{{"model.json", modelBytes}, {"reference.csv", corpusBytes}, {"annotations.template.csv", []byte("id,text,label\n")}} {
		if err = writeExclusive(filepath.Join(dir, f.name), f.data); err != nil {
			return nil, err
		}
	}
	if err = writeJSON(filepath.Join(dir, "manifest.json"), manifest); err != nil {
		return nil, err
	}
	return &Campaign{manifest, &m, corpus}, nil
}

func Load(dir, root string) (*Campaign, error) {
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	var manifest Manifest
	if err = json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	if manifest.Schema != 1 || manifest.NearDuplicateMaxDistance != 10 || manifest.TargetPrecision != 0.99 {
		return nil, fmt.Errorf("unsupported campaign protocol")
	}
	runtimeHash, err := RuntimeDigest(root)
	if err != nil {
		return nil, err
	}
	if runtimeHash != manifest.RuntimeSHA256 {
		return nil, fmt.Errorf("runtime/preprocessing changed since candidate freeze")
	}
	for _, f := range []struct{ name, hash string }{{"model.json", manifest.ModelSHA256}, {"reference.csv", manifest.CorpusSHA256}} {
		data, err = os.ReadFile(filepath.Join(dir, f.name))
		if err != nil {
			return nil, err
		}
		if digest(data) != f.hash {
			return nil, fmt.Errorf("%s changed since candidate freeze", f.name)
		}
	}
	m, err := model.Load(filepath.Join(dir, "model.json"))
	if err != nil {
		return nil, err
	}
	if err = validateModel(m); err != nil {
		return nil, err
	}
	if m.Threshold != manifest.Threshold || m.Version != manifest.ModelVersion {
		return nil, fmt.Errorf("model metadata does not match manifest")
	}
	corpus, err := readDocuments(filepath.Join(dir, "reference.csv"), true, true)
	if err != nil {
		return nil, err
	}
	return &Campaign{manifest, m, corpus}, nil
}
