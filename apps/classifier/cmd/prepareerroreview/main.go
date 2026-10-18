// prepareerroreview creates a blinded second-review packet from grouped OOF
// disagreements on the human training partition. It never changes labels or
// feeds the packet back into training.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/review"
)

const threshold = 0.5

type protocol struct {
	TrainIDs []string `json:"train_ids"`
}

type oofRow struct {
	ID     string             `json:"id"`
	Label  float64            `json:"label"`
	Fold   int                `json:"fold"`
	Scores map[string]float64 `json:"scores_by_candidate"`
}

type experimentProtocol struct {
	TrainIDs     []string `json:"training_ids"`
	ReviewedHash string   `json:"reviewed_labels_sha256"`
	ManifestHash string   `json:"frozen_manifest_sha256"`
	Folds        int      `json:"folds"`
	Model        struct {
		Threshold float64 `json:"fixed_threshold"`
	} `json:"model"`
}

type reviewRow struct {
	ID   string
	Text string
}

func verifyProvenance(p experimentProtocol, trainIDs map[string]bool, reviewedHash, manifestHash string) error {
	if p.ReviewedHash == "" || p.ReviewedHash != reviewedHash || p.ManifestHash == "" || p.ManifestHash != manifestHash {
		return fmt.Errorf("OOF experiment used a different reviewed-label or corpus-manifest snapshot")
	}
	if p.Folds != 5 || p.Model.Threshold != threshold {
		return fmt.Errorf("OOF experiment must use five folds and threshold %.1f", threshold)
	}
	seen := make(map[string]bool, len(p.TrainIDs))
	for _, id := range p.TrainIDs {
		if !trainIDs[id] || seen[id] {
			return fmt.Errorf("unexpected or duplicate OOF training id %q", id)
		}
		seen[id] = true
	}
	if len(seen) != len(trainIDs) {
		return fmt.Errorf("OOF training IDs do not match the frozen partition")
	}
	return nil
}

func hashFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func readReference(path string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read reference corpus: %w", err)
	}
	r := csv.NewReader(bytes.NewReader(b))
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read reference header: %w", err)
	}
	idCol, textCol := -1, -1
	for i, name := range header {
		if name == "id" {
			idCol = i
		}
		if name == "text" {
			textCol = i
		}
	}
	if idCol < 0 || textCol < 0 {
		return nil, fmt.Errorf("reference corpus needs id and text columns")
	}
	texts := make(map[string]string)
	for line := 2; ; line++ {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read reference row %d: %w", line, err)
		}
		if len(row) <= idCol || len(row) <= textCol || row[idCol] == "" {
			return nil, fmt.Errorf("malformed reference row %d", line)
		}
		if _, exists := texts[row[idCol]]; exists {
			return nil, fmt.Errorf("duplicate reference id %q", row[idCol])
		}
		texts[row[idCol]] = row[textCol]
	}
	return texts, nil
}

func selectDisagreements(rows []oofRow, trainIDs map[string]bool, labels map[string]int, texts map[string]string, excluded map[string]bool) ([]reviewRow, error) {
	seen := make(map[string]bool, len(rows))
	selected := make([]reviewRow, 0)
	for _, row := range rows {
		if row.ID == "" || seen[row.ID] {
			return nil, fmt.Errorf("empty or duplicate OOF id %q", row.ID)
		}
		seen[row.ID] = true
		if !trainIDs[row.ID] {
			return nil, fmt.Errorf("OOF id %s is outside the training partition", row.ID)
		}
		label, ok := labels[row.ID]
		if !ok || float64(label) != row.Label || (label != 0 && label != 1) {
			return nil, fmt.Errorf("OOF label does not match human label for %s", row.ID)
		}
		if row.Fold < 0 || row.Fold >= 5 {
			return nil, fmt.Errorf("invalid OOF fold for %s", row.ID)
		}
		score, ok := row.Scores["mixed_base"]
		if !ok || score < 0 || score > 1 {
			return nil, fmt.Errorf("missing or invalid mixed_base score for %s", row.ID)
		}
		if _, ok := texts[row.ID]; !ok {
			return nil, fmt.Errorf("OOF id %s is absent from reference texts", row.ID)
		}
		if excluded[row.ID] {
			return nil, fmt.Errorf("OOF id %s is explicitly excluded", row.ID)
		}
		if (score >= threshold) != (label == 1) {
			selected = append(selected, reviewRow{ID: row.ID, Text: texts[row.ID]})
		}
	}
	// Hash order keeps the annotation packet independent of labels, scores,
	// fold order, and the direction of disagreement.
	sort.Slice(selected, func(i, j int) bool {
		a := sha256.Sum256([]byte(selected[i].ID))
		b := sha256.Sum256([]byte(selected[j].ID))
		return string(a[:]) < string(b[:])
	})
	return selected, nil
}

func writeJSON(path string, value any) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		return err
	}
	return f.Sync()
}

func run() error {
	trainPath := flag.String("train-protocol", "data/score-investigation-20261013/protocol.json", "frozen human training ID protocol")
	oofPath := flag.String("oof", "data/cvocabulary-feature-families-20261013/oof-predictions.json", "grouped OOF predictions")
	experimentPath := flag.String("oof-protocol", "data/cvocabulary-feature-families-20261013/protocol.json", "protocol recorded with OOF predictions")
	reviewedPath := flag.String("reviewed", "data/reviewed_labels.csv", "human-reviewed labels used only to verify the OOF artifact")
	excludedPath := flag.String("excluded", "data/excluded_ids.csv", "IDs excluded from all training and review")
	referencePath := flag.String("reference", "data/confirmation-campaign/reference.csv", "frozen text corpus")
	manifestPath := flag.String("reference-manifest", "data/confirmation-campaign/manifest.json", "frozen corpus manifest")
	outDir := flag.String("out", "data/human-review-oof-errors-20261014", "new output directory for the blinded review packet")
	flag.Parse()

	protocolBytes, err := os.ReadFile(*trainPath)
	if err != nil {
		return fmt.Errorf("read train protocol: %w", err)
	}
	var p protocol
	if err := json.Unmarshal(protocolBytes, &p); err != nil {
		return fmt.Errorf("parse train protocol: %w", err)
	}
	trainIDs := make(map[string]bool, len(p.TrainIDs))
	for _, id := range p.TrainIDs {
		if id == "" || trainIDs[id] {
			return fmt.Errorf("empty or duplicate training id %q", id)
		}
		trainIDs[id] = true
	}
	if len(trainIDs) != 573 {
		return fmt.Errorf("expected 573 frozen training IDs, got %d", len(trainIDs))
	}
	reviewedHash, err := hashFile(*reviewedPath)
	if err != nil {
		return err
	}
	manifestHash, err := hashFile(*manifestPath)
	if err != nil {
		return err
	}
	experimentBytes, err := os.ReadFile(*experimentPath)
	if err != nil {
		return err
	}
	var experiment experimentProtocol
	if err := json.Unmarshal(experimentBytes, &experiment); err != nil {
		return err
	}
	if err := verifyProvenance(experiment, trainIDs, reviewedHash, manifestHash); err != nil {
		return err
	}
	referenceHash, err := hashFile(*referencePath)
	if err != nil {
		return err
	}
	manifestBytes, err := os.ReadFile(*manifestPath)
	if err != nil {
		return err
	}
	var corpusManifest struct {
		CorpusHash string `json:"corpus_sha256"`
	}
	if err := json.Unmarshal(manifestBytes, &corpusManifest); err != nil {
		return err
	}
	if corpusManifest.CorpusHash != referenceHash {
		return fmt.Errorf("reference corpus differs from its frozen manifest")
	}
	reviewedLabels, err := review.LoadReviewed(*reviewedPath)
	if err != nil {
		return err
	}
	labels := map[string]int{}
	for id, l := range reviewedLabels {
		labels[id] = l.Label
	}
	excluded, err := review.LoadExcluded(*excludedPath)
	if err != nil {
		return err
	}
	texts, err := readReference(*referencePath)
	if err != nil {
		return err
	}
	oofBytes, err := os.ReadFile(*oofPath)
	if err != nil {
		return fmt.Errorf("read OOF predictions: %w", err)
	}
	var rows []oofRow
	if err := json.Unmarshal(oofBytes, &rows); err != nil {
		return fmt.Errorf("parse OOF predictions: %w", err)
	}
	if len(rows) != 502 {
		return fmt.Errorf("expected 502 grouped OOF rows, got %d", len(rows))
	}
	selected, err := selectDisagreements(rows, trainIDs, labels, texts, excluded)
	if err != nil {
		return err
	}
	if len(selected) == 0 {
		return fmt.Errorf("no OOF disagreements to review")
	}
	if err := os.Mkdir(*outDir, 0700); err != nil {
		return fmt.Errorf("output directory must be new: %w", err)
	}
	file, err := os.OpenFile(filepath.Join(*outDir, "annotations.csv"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	w := csv.NewWriter(file)
	if err := w.Write([]string{"id", "text", "label", "reason"}); err != nil {
		file.Close()
		return err
	}
	for _, row := range selected {
		if err := w.Write([]string{row.ID, row.Text, "", ""}); err != nil {
			file.Close()
			return err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	protocolHash := sha256.Sum256(protocolBytes)
	oofHash := sha256.Sum256(oofBytes)
	experimentHash := sha256.Sum256(experimentBytes)
	manifest := map[string]any{
		"purpose": "blinded second human review of grouped OOF disagreements; training data only; not confirmation",
		"rows":    len(selected), "selection_threshold": threshold,
		"selection_source":            "mixed_base grouped five-fold out-of-fold predictions",
		"training_id_protocol_sha256": hex.EncodeToString(protocolHash[:]),
		"reviewed_labels_sha256":      reviewedHash,
		"reference_corpus_sha256":     referenceHash,
		"oof_protocol_sha256":         hex.EncodeToString(experimentHash[:]),
		"oof_predictions_sha256":      hex.EncodeToString(oofHash[:]),
		"annotation_fields":           map[string]string{"label": "1 if relevant CTI, 0 if not relevant, blank if ambiguous", "reason": "short independent rationale"},
		"blinding":                    "existing label, score, fold, and disagreement direction are omitted from annotations.csv",
		"created_by":                  "Codex; not human ground truth; do not import automatically",
	}
	if err := writeJSON(filepath.Join(*outDir, "manifest.json"), manifest); err != nil {
		return err
	}
	readme := "# Seconde revue aveugle des erreurs OOF\n\nCe lot contient des textes issus uniquement du partitionnement humain d'entraînement. Chaque texte avait été prédit hors pli et divergeait du label humain enregistré. Le CSV masque le label existant, le score, le pli et le sens du désaccord.\n\nRéannoter chaque texte selon la définition CTI du projet, inscrire un motif court dans `reason` et laisser `label` vide en cas d'ambiguïté. Cette seconde revue sert à détecter des désaccords de politique ou des erreurs de label; elle ne constitue pas une annotation indépendante de nouveaux documents. Ne pas importer les réponses automatiquement ni les compter comme confirmation finale.\n"
	if err := os.WriteFile(filepath.Join(*outDir, "README.md"), []byte(readme), 0600); err != nil {
		return err
	}
	fmt.Printf("Prepared %d blinded OOF disagreements in %s; no labels or scores included\n", len(selected), *outDir)
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
