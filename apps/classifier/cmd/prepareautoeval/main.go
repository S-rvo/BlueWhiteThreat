// prepareautoeval freezes an explicitly AI-referenced pilot campaign and
// prepares blind annotation splits. It never calls confirmfinal or scores data.
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"sort"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/confirmation"
)

type protocol struct {
	Mode                  string            `json:"mode"`
	LabelAuthority        string            `json:"label_authority"`
	SampleSize            int               `json:"sample_size"`
	ValidationSize        int               `json:"validation_size"`
	Seed                  int64             `json:"seed"`
	TargetRecall          float64           `json:"target_recall"`
	MinimumPrecision      float64           `json:"minimum_precision"`
	MinimumCoverage       float64           `json:"minimum_annotation_coverage"`
	MinimumPerClass       int               `json:"minimum_test_consensus_per_class"`
	InputHashes           map[string]string `json:"input_sha256"`
	MissingReferenceTexts int               `json:"reviewed_ids_without_reference_text"`
	Limitations           []string          `json:"limitations"`
}
type document struct {
	ID         string `json:"id"`
	Text       string `json:"text"`
	URL        string `json:"url"`
	Source     string `json:"source"`
	Language   string `json:"language"`
	Group      string `json:"group"`
	SourceType string `json:"source_type"`
	Created    string `json:"source_created_at"`
	Collected  string `json:"collected_at"`
}

func hash(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func write(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}
func writeLines(path string, docs []document) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, d := range docs {
		if err = enc.Encode(d); err != nil {
			return err
		}
	}
	return f.Sync()
}
func excludedIDs(path string) (map[string]bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 || len(rows[0]) == 0 || rows[0][0] != "id" {
		return nil, fmt.Errorf("review reference must start with id")
	}
	ids := map[string]bool{}
	for _, r := range rows[1:] {
		if len(r) > 0 {
			ids[r[0]] = true
		}
	}
	return ids, nil
}
func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	dir := flag.String("dir", "data/automatic-eval-2026-10-19/campaign", "new AI evaluation campaign")
	modelPath := flag.String("model", "model.json", "candidate to freeze")
	pool := flag.String("pool", "", "safe unscored pool JSONL; omit to freeze only")
	flag.Parse()
	const policyPath = "data/policies/cti-relevance-v1.json"
	const reviewedPath = "data/reviewed_labels.csv"
	const diagnosticPath = "../../docs/validation/nlp-synthetic-fixtures-2026-10-19.json"
	banned, err := excludedIDs(reviewedPath)
	if err != nil {
		return err
	}
	var p protocol
	if _, err = os.Stat(filepath.Join(*dir, "manifest.json")); os.IsNotExist(err) {
		docs, err := readReference("training_data.csv")
		if err != nil {
			return err
		}
		known := map[string]bool{}
		for _, d := range docs {
			known[d.ID] = true
		}
		missing := 0
		for id := range banned {
			if !known[id] {
				missing++
			}
		}
		b, err := os.ReadFile(diagnosticPath)
		if err != nil {
			return err
		}
		var diagnostics []confirmation.Document
		if err = json.Unmarshal(b, &diagnostics); err != nil {
			return err
		}
		for _, d := range diagnostics {
			if known[d.ID] {
				return fmt.Errorf("duplicate diagnostic id")
			}
			known[d.ID] = true
			docs = append(docs, d)
		}
		tmp, err := os.CreateTemp("", "bwt-auto-reference-*.csv")
		if err != nil {
			return err
		}
		defer os.Remove(tmp.Name())
		w := csv.NewWriter(tmp)
		_ = w.Write([]string{"id", "text", "label"})
		for _, d := range docs {
			if err = w.Write([]string{d.ID, d.Text, fmt.Sprintf("%.0f", d.Label)}); err != nil {
				tmp.Close()
				return err
			}
		}
		w.Flush()
		if err = w.Error(); err != nil {
			tmp.Close()
			return err
		}
		if err = tmp.Close(); err != nil {
			return err
		}
		p = protocol{Mode: "automatic_pilot", LabelAuthority: "AI consensus only; no human ground truth or certification", SampleSize: 400, ValidationSize: 200, Seed: 42, TargetRecall: .9, MinimumPrecision: .5, MinimumCoverage: .8, MinimumPerClass: 50, MissingReferenceTexts: missing, InputHashes: map[string]string{}, Limitations: []string{"Public Q&A pilot does not establish performance on Tor forums or access-sale posts.", "Two distinct annotation models must be actually available; absence is pending, not consensus.", "Known reviewed IDs with missing texts are excluded by ID; unidentified paraphrases cannot be checked.", "Framework manifest target_precision=0.99 belongs to the existing human-confirmation tool; this pilot uses automatic-protocol.json and never calls confirmfinal.", "SimHash is lexical, not proof against event-level leakage. Train, validation and test must remain separate."}}
		files := []string{"training_data.csv", reviewedPath, diagnosticPath, policyPath, "cmd/prepareautoeval/main.go", "../scraper/cmd/collecteval/main.go", "../../pkg/csamfilter/filter.go", "../../pkg/csamfilter/hosts.go"}
		for _, path := range files {
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			p.InputHashes[path] = hash(b)
		}
		if _, err = confirmation.Freeze(*dir, *modelPath, tmp.Name(), "."); err != nil {
			return err
		}
		if err = write(filepath.Join(*dir, "automatic-protocol.json"), p); err != nil {
			return err
		}
		log.Printf("Frozen automatic pilot with %d reference texts, %d additional reviewed IDs without texts", len(docs), missing)
	} else if err != nil {
		return err
	} else {
		b, err := os.ReadFile(filepath.Join(*dir, "automatic-protocol.json"))
		if err != nil {
			return err
		}
		if err = json.Unmarshal(b, &p); err != nil {
			return err
		}
		if p.Mode != "automatic_pilot" || p.SampleSize != 400 || p.ValidationSize != 200 || p.Seed != 42 {
			return fmt.Errorf("unsupported automatic protocol")
		}
		for path, want := range p.InputHashes {
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if hash(b) != want {
				return fmt.Errorf("frozen input changed: %s", path)
			}
		}
		if _, err = confirmation.Load(*dir, "."); err != nil {
			return err
		}
	}
	if *pool == "" {
		log.Print("Model/protocol frozen. No pool supplied; no annotations, metrics or certification fabricated.")
		return nil
	}
	f, err := os.Open(*pool)
	if err != nil {
		return err
	}
	defer f.Close()
	dec := json.NewDecoder(bufio.NewReader(f))
	byID := map[string]document{}
	for {
		var d document
		err = dec.Decode(&d)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if d.ID == "" || d.Text == "" || d.Group == "" || d.URL == "" || d.Source == "" || d.Language == "" || d.SourceType != "public_qa_pilot" || d.Collected == "" {
			return fmt.Errorf("missing document provenance")
		}
		if _, exists := byID[d.ID]; exists {
			return fmt.Errorf("duplicate pool id")
		}
		if banned[d.ID] {
			continue
		}
		byID[d.ID] = d
	}
	if len(byID) < p.SampleSize {
		return fmt.Errorf("pool too small: %d < %d; no selection made", len(byID), p.SampleSize)
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	tmp, err := os.CreateTemp("", "bwt-unscored-pool-*.csv")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	w := csv.NewWriter(tmp)
	_ = w.Write([]string{"id", "text"})
	for _, id := range ids {
		if err = w.Write([]string{id, byID[id].Text}); err != nil {
			tmp.Close()
			return err
		}
	}
	w.Flush()
	if err = w.Error(); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	selection, err := confirmation.Prepare(*dir, tmp.Name(), ".", p.SampleSize, p.Seed)
	if err != nil {
		return err
	}
	if len(selection.Documents) != p.SampleSize {
		return fmt.Errorf("selection sealed but insufficient: %d/%d; report inconclusive, do not silently resample", len(selection.Documents), p.SampleSize)
	}
	ann, err := os.Open(filepath.Join(*dir, "annotations.csv"))
	if err != nil {
		return err
	}
	rows, err := csv.NewReader(ann).ReadAll()
	ann.Close()
	if err != nil {
		return err
	}
	batch := make([]document, 0, len(rows)-1)
	groups := map[string]bool{}
	for _, r := range rows[1:] {
		d := byID[r[0]]
		if groups[d.Group] {
			return fmt.Errorf("thread repeated across selection")
		}
		groups[d.Group] = true
		batch = append(batch, d)
	}
	rand.New(rand.NewSource(p.Seed+1)).Shuffle(len(batch), func(i, j int) { batch[i], batch[j] = batch[j], batch[i] })
	if err = writeLines(filepath.Join(*dir, "validation.jsonl"), batch[:p.ValidationSize]); err != nil {
		return err
	}
	if err = writeLines(filepath.Join(*dir, "test.jsonl"), batch[p.ValidationSize:]); err != nil {
		return err
	}
	counts := map[string]map[string]int{"validation": {}, "test": {}}
	for i, d := range batch {
		split := "test"
		if i < p.ValidationSize {
			split = "validation"
		}
		counts[split][d.Source+":"+d.Language]++
	}
	poolBytes, err := os.ReadFile(*pool)
	if err != nil {
		return err
	}
	status := map[string]any{"mode": p.Mode, "status": "awaiting_two_distinct_AI_annotators", "selected": len(batch), "validation": p.ValidationSize, "test": len(batch) - p.ValidationSize, "pool_documents": len(byID), "pool_sha256": hash(poolBytes), "novelty_exclusions": selection.Excluded, "strata": counts, "model_scored": false, "human_ground_truth": false, "production_promotion": false, "reviewed_ids_without_reference_text": p.MissingReferenceTexts}
	if err = write(filepath.Join(*dir, "preparation.json"), status); err != nil {
		return err
	}
	log.Printf("Prepared %d validation + %d test documents. No model scores or AI labels produced.", p.ValidationSize, len(batch)-p.ValidationSize)
	return nil
}

// Historical blank texts still reserve their IDs; new pools forbid blank text.
func readReference(path string) ([]confirmation.Document, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) < 2 || len(rows[0]) != 3 || rows[0][0] != "id" || rows[0][1] != "text" || rows[0][2] != "label" {
		return nil, fmt.Errorf("invalid reference header")
	}
	seen := map[string]bool{}
	var docs []confirmation.Document
	for _, row := range rows[1:] {
		if len(row) != 3 || row[0] == "" || seen[row[0]] || (row[2] != "0" && row[2] != "1") {
			return nil, fmt.Errorf("invalid reference row")
		}
		seen[row[0]] = true
		label := 0.0
		if row[2] == "1" {
			label = 1
		}
		docs = append(docs, confirmation.Document{ID: row[0], Text: row[1], Label: label})
	}
	return docs, nil
}
