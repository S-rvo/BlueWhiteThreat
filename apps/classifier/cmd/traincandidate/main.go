// traincandidate fits a finite candidate family from historical labels and
// explicitly weak advisory labels. Only validation consensus selects parameters.
package main

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/classify"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/dedup"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/model"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/tfidf"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type document struct {
	ID             string  `json:"id"`
	Text           string  `json:"text"`
	Label          float64 `json:"label"`
	Source         string  `json:"source"`
	Group          string  `json:"group"`
	CollectedAt    string  `json:"collected_at"`
	SourceDate     string  `json:"source_date"`
	SourceActor    string  `json:"source_actor"`
	SamplingStream string  `json:"sampling_stream"`
}
type metric struct {
	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`
	F1        float64 `json:"f1"`
	TP        int     `json:"tp"`
	FP        int     `json:"fp"`
	FN        int     `json:"fn"`
	TN        int     `json:"tn"`
}
type choice struct {
	Threshold      float64 `json:"threshold"`
	L2             float64 `json:"l2"`
	FeatureConfig  string  `json:"feature_config,omitempty"`
	PositiveWeight float64 `json:"positive_weight_multiplier,omitempty"`
	Metrics        metric  `json:"metrics"`
	Feasible       bool    `json:"constraints_feasible"`
}

func measure(scores, labels []float64, t float64) metric {
	m := metric{}
	for i, s := range scores {
		if labels[i] == 1 {
			if s >= t {
				m.TP++
			} else {
				m.FN++
			}
		} else {
			if s >= t {
				m.FP++
			} else {
				m.TN++
			}
		}
	}
	if m.TP+m.FP > 0 {
		m.Precision = float64(m.TP) / float64(m.TP+m.FP)
	}
	if m.TP+m.FN > 0 {
		m.Recall = float64(m.TP) / float64(m.TP+m.FN)
	}
	if 2*m.TP+m.FP+m.FN > 0 {
		m.F1 = float64(2*m.TP) / float64(2*m.TP+m.FP+m.FN)
	}
	return m
}
func better(a, b choice) bool {
	if a.Feasible != b.Feasible {
		return a.Feasible
	}
	if a.Feasible {
		if a.Metrics.Precision != b.Metrics.Precision {
			return a.Metrics.Precision > b.Metrics.Precision
		}
		if a.Metrics.Recall != b.Metrics.Recall {
			return a.Metrics.Recall > b.Metrics.Recall
		}
	} else if a.Metrics.F1 != b.Metrics.F1 {
		return a.Metrics.F1 > b.Metrics.F1
	}
	if a.L2 != b.L2 {
		return a.L2 > b.L2
	}
	return a.Threshold > b.Threshold
}
func selectThreshold(scores, labels []float64, l2 float64) choice {
	best := choice{Threshold: 0, L2: l2, Metrics: measure(scores, labels, 0)}
	best.Feasible = best.Metrics.Precision >= .5 && best.Metrics.Recall >= .9
	for _, t := range append(scores, 1) {
		m := measure(scores, labels, t)
		c := choice{Threshold: t, L2: l2, Metrics: m, Feasible: m.Precision >= .5 && m.Recall >= .9}
		if better(c, best) {
			best = c
		}
	}
	return best
}
func readDocs(path string) ([]document, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	d := json.NewDecoder(f)
	var docs []document
	for {
		var x document
		e = d.Decode(&x)
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, e
		}
		docs = append(docs, x)
	}
	return docs, nil
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	csvPath := flag.String("historical", "training_data.csv", "historical labels")
	weakPath := flag.String("weak", "", "reserved weak-positive training JSONL")
	valPath := flag.String("validation", "", "held-out validation JSONL")
	consensusPath := flag.String("consensus", "", "complete blind validation consensus JSON")
	out := flag.String("out", "", "new private candidate JSON")
	crossFit := flag.Bool("cross-fit", false, "use validation consensus as development data with stratified 5-fold out-of-fold threshold selection")
	includeTargeted := flag.Bool("include-targeted-development", false, "explicitly use a lexically targeted development stream for OOF-only model selection; never qualifies a model")
	darkwebHead := flag.Bool("darkweb-head", false, "experimental fixed source-specific darkweb head")
	flag.Parse()
	if *out == "" || filepath.Base(*out) == "model.json" {
		return fmt.Errorf("explicit candidate path required; production name forbidden")
	}
	if _, e := os.Stat(*out); !os.IsNotExist(e) {
		return fmt.Errorf("candidate already exists or cannot be checked")
	}
	if *includeTargeted && !*crossFit {
		return fmt.Errorf("include-targeted-development requires --cross-fit")
	}
	f, e := os.Open(*csvPath)
	if e != nil {
		return e
	}
	rows, e := csv.NewReader(f).ReadAll()
	f.Close()
	if e != nil {
		return e
	}
	var ids, texts []string
	var labels []float64
	for _, r := range rows[1:] {
		if len(r) != 3 {
			return fmt.Errorf("invalid historical row")
		}
		if r[1] == "" {
			continue
		}
		label, e := strconv.ParseFloat(r[2], 64)
		if e != nil || (label != 0 && label != 1) {
			return fmt.Errorf("invalid historical label")
		}
		ids = append(ids, r[0])
		texts = append(texts, r[1])
		labels = append(labels, label)
	}
	weak, e := readDocs(*weakPath)
	if e != nil {
		return e
	}
	for _, d := range weak {
		if d.ID == "" || d.Text == "" || d.Label != 1 {
			return fmt.Errorf("invalid weak positive")
		}
		ids = append(ids, d.ID)
		texts = append(texts, d.Text)
		labels = append(labels, 1)
	}
	inputDocuments := len(texts)
	groups, conflicts := dedup.GroupByNormalizedText(ids, texts, labels)
	exactGroups := len(groups)
	groups, nearConflicts, mergeDiagnostics := dedup.MergeContentDuplicatesWithDiagnostics(groups)
	contentGroups := len(groups)
	texts = nil
	labels = nil
	for _, g := range groups {
		if len(g.Texts) > 0 && g.Texts[0] != "" {
			texts = append(texts, g.Texts[0])
			labels = append(labels, g.Label)
		}
	}
	validation, e := readDocs(*valPath)
	if e != nil {
		return e
	}
	var consensus struct {
		Documents []struct {
			ID      string `json:"id"`
			Status  string `json:"status"`
			TextSHA string `json:"text_sha256"`
			Label   *int   `json:"label"`
		} `json:"documents"`
	}
	raw, e := os.ReadFile(*consensusPath)
	if e != nil {
		return e
	}
	if e = json.Unmarshal(raw, &consensus); e != nil {
		return e
	}
	gold := map[string]int{}
	goldSHA := map[string]string{}
	for _, d := range consensus.Documents {
		if d.Status == "pending" {
			return fmt.Errorf("validation incomplete")
		}
		if d.Status == "consensus" {
			if d.Label == nil || (*d.Label != 0 && *d.Label != 1) {
				return fmt.Errorf("invalid validation label")
			}
			gold[d.ID] = *d.Label
			goldSHA[d.ID] = d.TextSHA
		}
	}
	var valIDs, valTexts, valSources, valGroups, valCollectedAt, valSamplingStreams []string
	var valLabels []float64
	for _, d := range validation {
		if label, ok := gold[d.ID]; ok {
			if fmt.Sprintf("%x", sha256.Sum256([]byte(d.Text))) != goldSHA[d.ID] {
				return fmt.Errorf("validation annotation text changed")
			}
			valIDs = append(valIDs, d.ID)
			valTexts = append(valTexts, d.Text)
			valSources = append(valSources, d.Source)
			group := d.Group
			if d.Source == "ransomware.live" && d.SourceActor != "" {
				group = "ransomware-actor:" + strings.ToLower(strings.TrimSpace(d.SourceActor))
			}
			valGroups = append(valGroups, group)
			valSamplingStreams = append(valSamplingStreams, d.SamplingStream)
			if d.SourceDate != "" {
				valCollectedAt = append(valCollectedAt, d.SourceDate)
			} else {
				valCollectedAt = append(valCollectedAt, d.CollectedAt)
			}
			valLabels = append(valLabels, float64(label))
		}
	}
	if len(valTexts) < 100 {
		return fmt.Errorf("insufficient validation consensus")
	}
	if *crossFit {
		winner, report, e := crossFitCandidate(texts, labels, valIDs, valTexts, valLabels, valSources, valGroups, valCollectedAt, valSamplingStreams, len(weak), *darkwebHead, *includeTargeted)
		if e != nil {
			return e
		}
		if e = model.SaveNew(*out, winner); e != nil {
			return e
		}
		report["annotation_input_documents"] = len(validation)
		report["annotation_consensus_documents"] = len(valTexts)
		report["annotation_consensus_coverage"] = float64(len(valTexts)) / float64(len(validation))
		report["near_duplicate_component_diagnostics"] = mergeDiagnostics
		rf, e := os.OpenFile(*out+".selection.json", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return e
		}
		defer rf.Close()
		return json.NewEncoder(rf).Encode(report)
	}
	// Keep calibration independent from hyperparameter/threshold selection.
	// The split is deterministic and stratified, so reruns use the same data.
	type ranked struct {
		index int
		key   string
	}
	calibrationIndex := map[int]bool{}
	for _, label := range []float64{0, 1} {
		var candidates []ranked
		for i := range valLabels {
			if valLabels[i] == label {
				candidates = append(candidates, ranked{i, fmt.Sprintf("%x", sha256.Sum256([]byte(valIDs[i])))})
			}
		}
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].key < candidates[j].key })
		n := len(candidates) / 5
		if n < 5 {
			return fmt.Errorf("insufficient validation examples per label for independent calibration")
		}
		for _, item := range candidates[:n] {
			calibrationIndex[item.index] = true
		}
	}
	selectionTexts, selectionLabels := []string{}, []float64{}
	calibrationTexts, calibrationLabels := []string{}, []float64{}
	for i := range valTexts {
		if calibrationIndex[i] {
			calibrationTexts = append(calibrationTexts, valTexts[i])
			calibrationLabels = append(calibrationLabels, valLabels[i])
		} else {
			selectionTexts = append(selectionTexts, valTexts[i])
			selectionLabels = append(selectionLabels, valLabels[i])
		}
	}
	valTexts, valLabels = selectionTexts, selectionLabels
	vectorizer := tfidf.Fit(texts, labels)
	X := make([][]float64, len(texts))
	for i, t := range texts {
		X[i] = vectorizer.Transform(t)
	}
	var winner *model.Model
	best := choice{Metrics: metric{F1: math.Inf(-1)}}
	var trials []choice
	version := "provisional-weak-advisories-" + time.Now().UTC().Format("20060102T150405Z")
	for _, l2 := range []float64{.001, .01, .1, .5} {
		lr := model.TrainWeighted(X, labels, model.BalancedWeights(labels), 4000, 2, l2)
		scores := make([]float64, len(valTexts))
		for i, t := range valTexts {
			scores[i] = lr.Predict(vectorizer.Transform(t))
		}
		chosen := selectThreshold(scores, valLabels, l2)
		trials = append(trials, chosen)
		if winner == nil || better(chosen, best) {
			best = chosen
			calibrationScores := make([]float64, len(calibrationTexts))
			for i, t := range calibrationTexts {
				calibrationScores[i] = lr.Predict(vectorizer.Transform(t))
			}
			winner = &model.Model{Vocabulary: vectorizer.Vocabulary, IDF: vectorizer.IDF, Weights: lr.Weights, Bias: lr.Bias, Threshold: chosen.Threshold, Calibration: model.FitPlattScaling(calibrationScores, calibrationLabels, 2000, .1), Version: version, Validation: &model.ValidationSummary{Documents: len(scores), Precision: chosen.Metrics.Precision, Recall: chosen.Metrics.Recall, F1: chosen.Metrics.F1, TargetPrecision: .5, MinimumRecall: .9, ObjectiveMet: chosen.Feasible, SelectionRule: "finite_l2_family_max_precision_at_recall_constraint", LabelAuthority: "two local AI models, not human ground truth"}}
		}
		fmt.Fprintf(os.Stderr, "Finished candidate l2=%g validation_f1=%.4f feasible=%t\n", l2, chosen.Metrics.F1, chosen.Feasible)
	}
	// Exercise the same loader/inference family that production uses before saving.
	_ = classify.New(winner).Classify(valTexts[0])
	if e = model.SaveNew(*out, winner); e != nil {
		return e
	}
	report := map[string]any{"selected": best, "trials": trials, "training_input_documents": inputDocuments, "training_documents": len(texts), "exact_groups_after_conflict_exclusion": exactGroups, "content_groups_after_conflict_exclusion": contentGroups, "exact_conflict_groups": len(conflicts), "content_duplicate_conflict_groups": len(nearConflicts), "near_duplicate_component_diagnostics": mergeDiagnostics, "same_label_groups_merged": exactGroups - contentGroups, "weak_advisories": len(weak), "selection_consensus": len(valTexts), "calibration_consensus": len(calibrationTexts), "calibration_reused_for_selection": false, "test_read": false, "production_promotion": false, "training_authority": "historical labels plus explicitly weak CVE-advisory positives"}
	reportFile, e := os.OpenFile(*out+".selection.json", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	defer reportFile.Close()
	return json.NewEncoder(reportFile).Encode(report)
}
