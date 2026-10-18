// cvclassweights compares fixed positive-class loss multipliers with grouped
// five-fold out-of-fold predictions from human-reviewed training examples.
// It does not read validation, dev, or confirmation labels and never saves a
// production model. Optional fitting saves a separate provisional candidate.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/dedup"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/model"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/overlap"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/review"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/tfidf"
)

const (
	folds              = 5
	seed         int64 = 20261014
	epochs             = 1200
	learningRate       = 2.0
	l2                 = 0.005
	cutoff             = 0.5
)

type group struct {
	id      string
	text    string
	label   float64
	members []string
}

// transformTerms keeps the fitted vocabulary and IDF unchanged. Binary mode
// removes within-document frequency only; it remains an experiment in this CLI.
func transformTerms(v *tfidf.Vectorizer, doc, mode string) []float64 {
	if mode == "saturated" {
		return v.TransformSaturated(doc)
	}
	x := v.Transform(doc)
	if mode == "log" {
		return x
	}
	var sum float64
	for i := range x {
		if x[i] != 0 {
			x[i] = v.IDF[i]
			sum += x[i] * x[i]
		}
	}
	if sum > 0 {
		norm := math.Sqrt(sum)
		for i := range x {
			x[i] /= norm
		}
	}
	return x
}

// transformBlocks averages normalized block vectors and normalizes the result.
// Vocabulary/IDF still come from whole training documents, never heldout blocks.
func transformBlocks(v *tfidf.Vectorizer, doc, mode string, blockWords int) []float64 {
	if blockWords == 0 {
		return transformTerms(v, doc, mode)
	}
	words := strings.Fields(doc)
	if len(words) <= blockWords {
		return transformTerms(v, doc, mode)
	}
	pooled := make([]float64, len(v.Vocabulary))
	for start := 0; start < len(words); start += blockWords {
		end := start + blockWords
		if end > len(words) {
			end = len(words)
		}
		x := transformTerms(v, strings.Join(words[start:end], " "), mode)
		for i, val := range x {
			pooled[i] += val
		}
	}
	var sum float64
	for _, val := range pooled {
		sum += val * val
	}
	if sum > 0 {
		norm := math.Sqrt(sum)
		for i := range pooled {
			pooled[i] /= norm
		}
	}
	return pooled
}

// deduplicateLongLines is experimental and label-independent. Short headings
// and table cells remain; only repeated lines with at least five words vanish.
func deduplicateLongLines(doc string) string {
	seen := make(map[string]bool)
	lines := strings.Split(doc, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		words := strings.Fields(line)
		if len(words) >= 5 {
			key := strings.ToLower(strings.Join(words, " "))
			if seen[key] {
				continue
			}
			seen[key] = true
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

type metric struct {
	N                int     `json:"n"`
	TP               int     `json:"tp"`
	FP               int     `json:"fp"`
	TN               int     `json:"tn"`
	FN               int     `json:"fn"`
	Precision        float64 `json:"precision"`
	Recall           float64 `json:"recall"`
	F1               float64 `json:"f1"`
	Accuracy         float64 `json:"accuracy"`
	AveragePrecision float64 `json:"average_precision"`
	Brier            float64 `json:"brier"`
}

func writeNew(path string, value any) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	e := json.NewEncoder(f)
	e.SetIndent("", "  ")
	if err := e.Encode(value); err != nil {
		return err
	}
	return f.Sync()
}

func metrics(scores, labels []float64) metric {
	m := metric{N: len(scores)}
	for i, score := range scores {
		actual := labels[i] == 1
		predicted := score >= cutoff
		switch {
		case predicted && actual:
			m.TP++
		case predicted && !actual:
			m.FP++
		case !predicted && !actual:
			m.TN++
		default:
			m.FN++
		}
		m.Brier += (score - labels[i]) * (score - labels[i])
	}
	if m.N > 0 {
		m.Brier /= float64(m.N)
		m.Accuracy = float64(m.TP+m.TN) / float64(m.N)
	}
	if m.TP+m.FP > 0 {
		m.Precision = float64(m.TP) / float64(m.TP+m.FP)
	}
	if m.TP+m.FN > 0 {
		m.Recall = float64(m.TP) / float64(m.TP+m.FN)
	}
	if m.Precision+m.Recall > 0 {
		m.F1 = 2 * m.Precision * m.Recall / (m.Precision + m.Recall)
	}
	// Average precision from ranked out-of-fold scores.
	order := make([]int, len(scores))
	positives := 0
	for i := range order {
		order[i] = i
		if labels[i] == 1 {
			positives++
		}
	}
	sort.Slice(order, func(i, j int) bool { return scores[order[i]] > scores[order[j]] })
	hits, seen, sum := 0, 0, 0.0
	for start := 0; start < len(order); {
		end := start + 1
		groupHits := 0
		for end < len(order) && scores[order[end]] == scores[order[start]] {
			end++
		}
		for _, idx := range order[start:end] {
			if labels[idx] == 1 {
				groupHits++
			}
		}
		hits += groupHits
		seen += end - start
		if positives > 0 {
			sum += float64(groupHits) / float64(positives) * float64(hits) / float64(seen)
		}
		start = end
	}
	if positives > 0 {
		m.AveragePrecision = sum
	}
	return m
}

func trainingSubset(groups []group, assigned []int, heldout int, fraction float64, sampleSeed int64) []int {
	byLabel := [2][]int{}
	for i, g := range groups {
		if assigned[i] != heldout {
			byLabel[int(g.label)] = append(byLabel[int(g.label)], i)
		}
	}
	var selected []int
	for _, indices := range byLabel {
		if fraction < 1 {
			sort.Slice(indices, func(i, j int) bool {
				a := sha256.Sum256([]byte(fmt.Sprintf("%d:%s", sampleSeed, groups[indices[i]].id)))
				b := sha256.Sum256([]byte(fmt.Sprintf("%d:%s", sampleSeed, groups[indices[j]].id)))
				return bytes.Compare(a[:], b[:]) < 0
			})
			n := int(float64(len(indices)) * fraction)
			if n == 0 && len(indices) > 0 {
				n = 1
			}
			indices = indices[:n]
		}
		selected = append(selected, indices...)
	}
	sort.Ints(selected)
	return selected
}

func run() error {
	out := flag.String("out", "data/cvclassweights-20261014", "new result directory")
	campaignDir := flag.String("campaign", "data/confirmation-campaign", "frozen corpus campaign")
	protocolPath := flag.String("protocol", "data/score-investigation-20261013/protocol.json", "frozen human train ID protocol")
	reviewedPath := flag.String("reviewed", "data/reviewed_labels.csv", "human labels CSV")
	cleaning := flag.String("text-cleaning", "none", "experimental preprocessing: none or deduplicate-long-lines")
	balancedOnly := flag.Bool("balanced-only", false, "run only the balanced loss baseline (multiplier=1)")
	foldSeed := flag.Int64("seed", seed, "deterministic grouped fold assignment seed")
	trainFraction := flag.Float64("train-fraction", 1, "stratified fraction of each fold's training groups; heldout unchanged")
	dynamicLines := flag.Bool("dynamic-lines", false, "experimental grouping: also match repeated interfaces by lines")
	termMode := flag.String("term-frequency", "log", "experimental term frequency: log, binary or saturated")
	blockWords := flag.Int("block-words", 0, "experimental normalized block pooling; 0 keeps whole-document transform")
	wordReserve := flag.Int("word-reserve", 0, "experimental unigram slots within the unchanged 5000-feature cap")
	scriptReserve := flag.Int("script-reserve", 0, "experimental non-Latin feature slots, minimum frequency 3; OOF only")
	minDF := flag.Int("min-df", 1, "experimental minimum training-document frequency before vocabulary ranking")
	familyPath := flag.String("fold-families", "", "experimental JSON arrays of representative IDs constrained to the same fold")
	fitCandidate := flag.Bool("fit-candidate", false, "also save a provisional full-train candidate compatible with the production vectorizer")
	embeddingPath := flag.String("embeddings", "", "experimental immutable train-only embedding cache vectors.json")
	hybrid := flag.Bool("hybrid", false, "experimental equal-weight concatenation of TF-IDF and frozen embeddings")
	ratioWeighting := flag.Bool("log-count-ratio", false, "experimental fold-train document-presence log-count ratios, alpha=1, followed by L2 normalization")
	loss := flag.String("loss", "logistic", "experimental training objective: logistic, squared-hinge or capped-hinge (margin step 0.1)")
	familyNormalization := flag.Bool("normalize-feature-families", false, "experimental equal L2 mass per nonzero word/bigram/character/structural namespace")
	assistantPath := flag.String("experimental-assistant-train-labels", "", "adaptive sensitivity experiment: assistant labels in fold training only; original OOF labels unchanged; no export")
	abstentionPath := flag.String("experimental-assistant-abstentions", "", "adaptive sensitivity: omit abstained duplicate groups from fold training only; retain all original labels; no export")
	flag.Parse()
	if *assistantPath != "" && *abstentionPath != "" {
		return fmt.Errorf("choose label sensitivity or abstention exclusion, not both")
	}
	if (*assistantPath != "" || *abstentionPath != "") && (*fitCandidate || !*balancedOnly || *embeddingPath != "" || *dynamicLines || *trainFraction != 1 || *ratioWeighting || *familyNormalization || *scriptReserve != 0 || *wordReserve != 0 || *blockWords != 0 || *termMode != "log" || *cleaning != "none" || (*loss != "logistic" && *loss != "squared-hinge")) {
		return fmt.Errorf("assistant sensitivity requires unchanged balanced lexical OOF, no other experiment or export")
	}
	if *scriptReserve < 0 || *scriptReserve > tfidf.MaxVocabularySize {
		return fmt.Errorf("invalid script reserve")
	}
	if *scriptReserve > 0 && (*fitCandidate || *wordReserve != 0 || *embeddingPath != "" || *ratioWeighting || *familyNormalization || !*balancedOnly || *minDF != 10) {
		return fmt.Errorf("script reserve requires balanced lexical OOF, minDF10, no export or other vocabulary experiments")
	}
	if *familyNormalization && (*embeddingPath != "" || *ratioWeighting || *fitCandidate || !*balancedOnly || *blockWords != 0 || *termMode != "log") {
		return fmt.Errorf("feature family normalization supports balanced lexical OOF with log TF only; no export, embeddings, ratios or blocks")
	}
	if *loss != "logistic" && *loss != "squared-hinge" && *loss != "capped-hinge" {
		return fmt.Errorf("unknown loss %q", *loss)
	}
	if *loss == "capped-hinge" && (*fitCandidate || *assistantPath != "" || *familyNormalization || *dynamicLines || *scriptReserve != 0 || *wordReserve != 0 || *blockWords != 0 || *cleaning != "none" || *termMode != "log" || *trainFraction != 1) {
		return fmt.Errorf("capped-hinge supports unchanged lexical human-label OOF only, without export or another experiment")
	}
	if *loss != "logistic" && (*embeddingPath != "" || *ratioWeighting || !*balancedOnly) {
		return fmt.Errorf("margin objectives require balanced-only lexical training, no embeddings or ratio weighting")
	}
	if *ratioWeighting && (*embeddingPath != "" || *fitCandidate) {
		return fmt.Errorf("log-count-ratio supports lexical OOF only; candidate export and embeddings are unsupported")
	}
	if *hybrid && *embeddingPath == "" {
		return fmt.Errorf("hybrid requires embeddings")
	}
	if *embeddingPath != "" && (*fitCandidate || !*balancedOnly || *cleaning != "none" || *dynamicLines || *trainFraction != 1 || *blockWords != 0 || *wordReserve != 0 || *termMode != "log") {
		return fmt.Errorf("embeddings require balanced-only and unchanged full-train text; candidate export is unsupported")
	}
	if *wordReserve < 0 || *wordReserve > tfidf.MaxVocabularySize {
		return fmt.Errorf("invalid word-reserve")
	}
	if *fitCandidate && (*wordReserve != 0 || !*balancedOnly || *cleaning != "none" || *termMode != "log" || *blockWords != 0 || *trainFraction != 1 || *dynamicLines) {
		return fmt.Errorf("fit-candidate requires balanced-only, unmodified log TF, no blocks or dynamic grouping, and full train fraction")
	}
	if *minDF < 1 {
		return fmt.Errorf("min-df must be positive")
	}
	if *blockWords < 0 {
		return fmt.Errorf("block-words must be nonnegative")
	}
	if *termMode != "log" && *termMode != "binary" && *termMode != "saturated" {
		return fmt.Errorf("invalid term-frequency %q", *termMode)
	}
	if *termMode == "saturated" && (*loss != "squared-hinge" || !*balancedOnly || *embeddingPath != "" || *ratioWeighting || *familyNormalization || *scriptReserve != 0 || *wordReserve != 0 || *blockWords != 0 || *cleaning != "none" || *dynamicLines || *trainFraction != 1 || *fitCandidate || *assistantPath != "") {
		return fmt.Errorf("saturated TF supports unchanged squared-hinge human-label lexical OOF only; no export or other experiment")
	}
	if math.IsNaN(*trainFraction) || *trainFraction <= 0 || *trainFraction > 1 {
		return fmt.Errorf("train fraction must be in (0,1]")
	}
	if *cleaning != "none" && *cleaning != "deduplicate-long-lines" {
		return fmt.Errorf("unknown text cleaning %q", *cleaning)
	}
	// Read the frozen text snapshot directly. This is an exploratory training
	// experiment, not a confirmation run; campaign runtime validation is
	// intentionally not invoked and no confirmation data is opened.
	referenceBytes, err := os.ReadFile(filepath.Join(*campaignDir, "reference.csv"))
	if err != nil {
		return err
	}
	manifestBytes, err := os.ReadFile(filepath.Join(*campaignDir, "manifest.json"))
	if err != nil {
		return err
	}
	var frozenManifest struct {
		CorpusSHA256 string `json:"corpus_sha256"`
	}
	if err = json.Unmarshal(manifestBytes, &frozenManifest); err != nil {
		return err
	}
	if fmt.Sprintf("%x", sha256.Sum256(referenceBytes)) != frozenManifest.CorpusSHA256 {
		return fmt.Errorf("frozen reference corpus hash mismatch")
	}
	// Parse only ID and text columns. Never parse labels from the frozen corpus,
	// which also contains reserved validation/dev/test rows.
	csvReader := csv.NewReader(bytes.NewReader(referenceBytes))
	csvReader.FieldsPerRecord = -1
	records, err := csvReader.ReadAll()
	if err != nil {
		return err
	}
	if len(records) == 0 || len(records[0]) < 2 {
		return fmt.Errorf("frozen reference corpus has no usable header")
	}
	idColumn, textColumn := -1, -1
	for i, name := range records[0] {
		switch name {
		case "id":
			idColumn = i
		case "text":
			textColumn = i
		}
	}
	if idColumn < 0 || textColumn < 0 {
		return fmt.Errorf("frozen reference corpus must contain id and text columns")
	}
	textByID := make(map[string]string, len(records)-1)
	for _, record := range records[1:] {
		if len(record) <= idColumn || len(record) <= textColumn {
			return fmt.Errorf("malformed frozen corpus row")
		}
		textByID[record[idColumn]] = record[textColumn]
	}
	var protocol struct {
		TrainIDs []string `json:"train_ids"`
	}
	b, err := os.ReadFile(*protocolPath)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(b, &protocol); err != nil {
		return err
	}
	reviewedLabels, err := review.LoadReviewed(*reviewedPath)
	if err != nil {
		return err
	}
	reviewed := map[string]int{}
	for id, l := range reviewedLabels {
		reviewed[id] = l.Label
	}
	var ids, texts []string
	var labels []float64
	for _, id := range protocol.TrainIDs {
		label, ok := reviewed[id]
		if !ok {
			continue
		}
		doc, exists := textByID[id]
		if !exists {
			return fmt.Errorf("training id %s absent from frozen corpus", id)
		}
		ids = append(ids, id)
		texts = append(texts, doc)
		labels = append(labels, float64(label))
	}
	if len(ids) != 573 {
		return fmt.Errorf("expected 573 human training docs, got %d", len(ids))
	}
	var assistantLabels map[string]float64
	var assistantSHA string
	var abstentions map[string]bool
	var abstentionSHA string
	if *abstentionPath != "" {
		_, abstentions, abstentionSHA, err = loadAssistantReview(*abstentionPath, ids, texts, labels)
		if err != nil {
			return err
		}
		if len(abstentions) == 0 {
			return fmt.Errorf("no explicit assistant abstentions")
		}
	}
	if *assistantPath != "" {
		assistantLabels, assistantSHA, err = loadAssistantLabels(*assistantPath, ids, texts, labels)
		if err != nil {
			return err
		}
	}
	exact, exactConflicts := dedup.GroupByNormalizedText(ids, texts, labels)
	linePrints := make([]linePrint, len(exact))
	var dynamicPairs [][2]string
	prints := make([]overlap.Print, len(exact))
	for i, g := range exact {
		prints[i] = overlap.Fingerprint(g.Texts[0])
		if *dynamicLines {
			linePrints[i] = fingerprintLines(g.Texts[0])
		}
	}
	parent := make([]int, len(exact))
	for i := range parent {
		parent[i] = i
	}
	var root func(int) int
	root = func(i int) int {
		if parent[i] != i {
			parent[i] = root(parent[i])
		}
		return parent[i]
	}
	for i := range exact {
		for j := i + 1; j < len(exact); j++ {
			lexicalMatch := overlap.Similar(prints[i], prints[j])
			lineMatch := *dynamicLines && similarLines(linePrints[i], linePrints[j])
			if lineMatch && !lexicalMatch {
				dynamicPairs = append(dynamicPairs, [2]string{exact[i].IDs[0], exact[j].IDs[0]})
			}
			if lexicalMatch || lineMatch {
				a, b := root(i), root(j)
				if a != b {
					parent[b] = a
				}
			}
		}
	}
	clusters := map[int][]int{}
	var clusterOrder []int
	for i := range exact {
		r := root(i)
		if _, ok := clusters[r]; !ok {
			clusterOrder = append(clusterOrder, r)
		}
		clusters[r] = append(clusters[r], i)
	}
	groups := make([]group, 0, len(clusterOrder))
	nearConflicts := 0
	for _, r := range clusterOrder {
		members := clusters[r]
		label := exact[members[0]].Label
		consistent := true
		for _, idx := range members {
			if exact[idx].Label != label {
				consistent = false
			}
		}
		if !consistent {
			nearConflicts++
			continue
		}
		var memberIDs []string
		for _, idx := range members {
			memberIDs = append(memberIDs, exact[idx].IDs...)
		}
		groups = append(groups, group{id: exact[members[0]].IDs[0], text: exact[members[0]].Texts[0], label: label, members: memberIDs})
	}
	if !*dynamicLines && len(groups) != 499 {
		return fmt.Errorf("expected 499 consistent groups after deduplication, got %d", len(groups))
	}
	if *cleaning == "deduplicate-long-lines" {
		prints := make([]overlap.Print, len(groups))
		cleanedLines := make([]linePrint, len(groups))
		for i := range groups {
			groups[i].text = deduplicateLongLines(groups[i].text)
			prints[i] = overlap.Fingerprint(groups[i].text)
			if *dynamicLines {
				cleanedLines[i] = fingerprintLines(groups[i].text)
			}
		}
		// Reject cleaning-induced overlap between groups, preserving the same
		// split as the raw baseline rather than leaking equivalent held-out text.
		for i := range groups {
			for j := i + 1; j < len(groups); j++ {
				if overlap.Similar(prints[i], prints[j]) || (*dynamicLines && similarLines(cleanedLines[i], cleanedLines[j])) {
					return fmt.Errorf("cleaning makes groups %d and %d near-duplicates; common regrouping required", i, j)
				}
			}
		}
	}
	byClass := [2][]int{}
	for i, g := range groups {
		byClass[int(g.label)] = append(byClass[int(g.label)], i)
	}
	rng := rand.New(rand.NewSource(*foldSeed))
	assigned := make([]int, len(groups))
	for label := range byClass {
		rng.Shuffle(len(byClass[label]), func(i, j int) { byClass[label][i], byClass[label][j] = byClass[label][j], byClass[label][i] })
		for i, idx := range byClass[label] {
			assigned[idx] = i % folds
		}
	}
	var familyBytes []byte
	if *familyPath != "" {
		familyBytes, err = os.ReadFile(*familyPath)
		if err != nil {
			return err
		}
		var families [][]string
		if err = json.Unmarshal(familyBytes, &families); err != nil {
			return err
		}
		if err = constrainFamilies(groups, assigned, families); err != nil {
			return err
		}
	}
	multipliers := []float64{0.125, 0.25, 0.5, 1.0, 2.0}
	if *balancedOnly {
		multipliers = []float64{1}
	}
	scores := make([][]float64, len(multipliers))
	ys := make([]float64, len(groups))
	groupIDs := make([]string, len(groups))
	foldsForGroup := make([]int, len(groups))
	for i, g := range groups {
		groupIDs[i] = g.id
		ys[i] = g.label
		foldsForGroup[i] = assigned[i]
	}
	trainingIDs := make([][]string, folds)
	var embeddings map[string][]float64
	var embeddingSHA string
	if *embeddingPath != "" {
		embeddings, embeddingSHA, err = loadEmbeddings(*embeddingPath, groups)
		if err != nil {
			return err
		}
	}
	for fold := 0; fold < folds; fold++ {
		var trainTexts []string
		var trainY []float64
		var heldout []int
		for i := range groups {
			if assigned[i] == fold {
				heldout = append(heldout, i)
			}
		}
		for _, i := range trainingSubset(groups, assigned, fold, *trainFraction, *foldSeed) {
			g := groups[i]
			if abstainedGroup(g, abstentions) {
				continue
			}
			trainTexts = append(trainTexts, g.text)
			trainY = append(trainY, foldTrainingLabel(g, assistantLabels))
			trainingIDs[fold] = append(trainingIDs[fold], g.id)
		}
		var v *tfidf.Vectorizer
		if embeddings == nil || *hybrid {
			var fitErr error
			if *scriptReserve > 0 {
				v, fitErr = tfidf.FitWithScriptReserve(trainTexts, trainY, *minDF, *scriptReserve, 3)
			} else {
				v, fitErr = tfidf.FitWithWordReserve(trainTexts, trainY, *minDF, *wordReserve)
			}
			if fitErr != nil {
				return fitErr
			}
		}
		var namespaces []int
		if *familyNormalization {
			namespaces = featureFamilies(v.Vocabulary)
		}
		X := make([][]float64, len(trainTexts))
		for i, doc := range trainTexts {
			if embeddings != nil {
				X[i] = embeddings[trainingIDs[fold][i]]
				if *hybrid {
					X[i] = combineVectors(transformBlocks(v, doc, *termMode, *blockWords), X[i])
				}
			} else {
				X[i] = transformBlocks(v, doc, *termMode, *blockWords)
			}
		}
		if *familyNormalization {
			for i := range X {
				X[i] = normalizeFamilies(X[i], namespaces)
			}
		}
		var ratios []float64
		if *ratioWeighting {
			ratios, err = fitLogCountRatios(X, trainY)
			if err != nil {
				return err
			}
			for i := range X {
				X[i] = applyLogCountRatios(X[i], ratios)
			}
		}
		balanced := model.BalancedWeights(trainY)
		for ci, mult := range multipliers {
			weights := append([]float64(nil), balanced...)
			for i, label := range trainY {
				if label == 1 {
					weights[i] *= mult
				}
			}
			lr, fitErr := fitObjective(X, trainY, weights, *loss)
			if fitErr != nil {
				return fitErr
			}
			if scores[ci] == nil {
				scores[ci] = make([]float64, len(groups))
			}
			for _, idx := range heldout {
				input := embeddings[groups[idx].id]
				if embeddings == nil {
					input = transformBlocks(v, groups[idx].text, *termMode, *blockWords)
				} else if *hybrid {
					input = combineVectors(transformBlocks(v, groups[idx].text, *termMode, *blockWords), input)
				}
				if *ratioWeighting {
					input = applyLogCountRatios(input, ratios)
				}
				if *familyNormalization {
					input = normalizeFamilies(input, namespaces)
				}
				scores[ci][idx] = lr.Predict(input)
			}
			log.Printf("fold=%d positive_multiplier=%.3g train=%d heldout=%d", fold, mult, len(trainY), len(heldout))
		}
	}
	if err = os.Mkdir(*out, 0700); err != nil {
		return err
	}
	result := map[string]metric{}
	for i, mult := range multipliers {
		result[fmt.Sprintf("positive_multiplier_%g", mult)] = metrics(scores[i], ys)
	}
	labelsBytes, err := os.ReadFile(*reviewedPath)
	if err != nil {
		return err
	}
	protocolBytes, err := os.ReadFile(*protocolPath)
	if err != nil {
		return err
	}
	protocolOut := map[string]any{"purpose": "grouped five-fold OOF diagnostic; no validation/dev/test/confirmation scores used", "training_documents": len(ids), "consistent_groups": len(groups), "exact_conflicts_excluded": len(exactConflicts), "near_conflicts_excluded": nearConflicts, "grouping": "normalized exact text, then connected components of overlap.Similar (five-word shingle Jaccard >= 0.80, minimum 20 tokens)", "folds": folds, "seed": *foldSeed, "l2": l2, "epochs": epochs, "learning_rate": learningRate, "fixed_threshold": cutoff, "positive_multipliers": multipliers, "reviewed_sha256": fmt.Sprintf("%x", sha256.Sum256(labelsBytes)), "frozen_manifest_sha256": fmt.Sprintf("%x", sha256.Sum256(manifestBytes)), "frozen_corpus_sha256": frozenManifest.CorpusSHA256, "training_id_protocol_sha256": fmt.Sprintf("%x", sha256.Sum256(protocolBytes))}
	protocolOut["normalize_feature_families"] = *familyNormalization
	protocolOut["family_normalization_rule"] = "each nonzero namespace unit L2, then global unit L2; words/bigrams/characters/structural; same fold-fitted vocabulary"
	protocolOut["loss"] = *loss
	if *abstentionPath != "" {
		protocolOut["assistant_abstentions_sha256"] = abstentionSHA
		protocolOut["assistant_abstention_rule"] = "adaptive assistant abstentions omit entire exact/near duplicate groups from fold training only; unchanged original labels, grouping, folds and heldout IDs; no export"
		excluded := []string{}
		for _, g := range groups {
			if abstainedGroup(g, abstentions) {
				excluded = append(excluded, g.id)
			}
		}
		protocolOut["assistant_excluded_group_ids"] = excluded
		protocolOut["independent_confirmation"] = false
	}
	if *assistantPath != "" {
		protocolOut["assistant_train_labels_sha256"] = assistantSHA
		protocolOut["assistant_train_label_rule"] = "adaptive provisional assistant labels for fold-training representatives only; original human grouping, folds and heldout labels unchanged; nonrepresentatives never counted twice"
		changed := []string{}
		for _, g := range groups {
			if foldTrainingLabel(g, assistantLabels) != g.label {
				changed = append(changed, g.id)
			}
		}
		protocolOut["assistant_changed_representative_ids"] = changed
		protocolOut["independent_confirmation"] = false
	}
	if *loss == "squared-hinge" || *loss == "capped-hinge" {
		protocolOut["learning_rate"] = 0.1
		protocolOut["score_semantics"] = "sigmoid of margin, not calibrated probability"
	}
	if *loss == "capped-hinge" {
		protocolOut["capped_hinge_rule"] = "L(m)=0.5-m for m<0; 0.5*(1-m)^2 for 0<=m<1; 0 for m>=1; fixed derivative cap 1 before sample weighting"
	}
	protocolOut["log_count_ratio"] = *ratioWeighting
	protocolOut["log_count_ratio_rule"] = "fold-train document presence; additive smoothing alpha=1; class feature totals; signed log ratio; final L2 normalization"
	protocolOut["dynamic_lines"] = *dynamicLines
	if embeddings != nil {
		protocolOut["representation"] = "frozen local encoder embeddings; no encoder fine-tuning; identity pinned by embedding cache protocol"
		protocolOut["embedding_cache_sha256"] = embeddingSHA
		protocolOut["embedding_dimensions"] = 768
		protocolOut["hybrid"] = *hybrid
		if *hybrid {
			protocolOut["representation"] = "equal-weight concatenation of fold-fitted TF-IDF and frozen local encoder embeddings; final L2 normalization"
		}
	}
	protocolOut["dynamic_line_rule"] = "at least 30 distinct lines each; line Jaccard >=0.90; common distinct-line words >=0.80 of each text"
	protocolOut["dynamic_additional_pairs"] = dynamicPairs
	protocolOut["text_cleaning"] = *cleaning
	protocolOut["term_frequency"] = *termMode
	if *termMode == "saturated" {
		protocolOut["term_frequency_rule"] = "count/(count+1) times fold-fitted IDF, then L2 normalization; fixed saturation constant 1"
	}
	protocolOut["min_document_frequency"] = *minDF
	protocolOut["word_reserve"] = *wordReserve
	protocolOut["script_reserve"] = *scriptReserve
	protocolOut["script_reserve_rare_minimum"] = 3
	if len(familyBytes) > 0 {
		protocolOut["fold_families_sha256"] = fmt.Sprintf("%x", sha256.Sum256(familyBytes))
		protocolOut["fold_family_rule"] = "connected families share the original fold of their lexicographically smallest representative ID; documents and labels retained"
	}
	protocolOut["block_words"] = *blockWords
	protocolOut["block_pooling"] = "sum L2-normalized block vectors, then L2-normalize; whole-training-document vocabulary and IDF"
	protocolOut["train_fraction"] = *trainFraction
	protocolOut["fold_training_ids"] = trainingIDs
	protocolOut["cleaning_overlap_guard"] = "reject additional exact or near-duplicate groups after preprocessing"
	protocolOut["candidate_fit_requested"] = *fitCandidate
	if err = writeNew(filepath.Join(*out, "protocol.json"), protocolOut); err != nil {
		return err
	}
	if err = writeNew(filepath.Join(*out, "metrics.json"), result); err != nil {
		return err
	}
	if err = writeNew(filepath.Join(*out, "oof.json"), map[string]any{"ids": groupIDs, "labels": ys, "folds": foldsForGroup, "scores": scores}); err != nil {
		return err
	}
	if *fitCandidate {
		if err = saveFullTrainCandidate(*out, groups, *minDF, *loss); err != nil {
			return err
		}
	}
	for _, mult := range multipliers {
		m := result[fmt.Sprintf("positive_multiplier_%g", mult)]
		log.Printf("OOF multiplier=%.3g n=%d P=%.3f R=%.3f F1=%.3f AP=%.3f Brier=%.4f TP/FP/TN/FN=%d/%d/%d/%d", mult, m.N, m.Precision, m.Recall, m.F1, m.AveragePrecision, m.Brier, m.TP, m.FP, m.TN, m.FN)
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
