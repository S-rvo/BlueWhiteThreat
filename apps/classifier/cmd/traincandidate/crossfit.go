package main

import (
	"fmt"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/dedup"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/model"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/overlap"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/tfidf"
	"math"
	"os"
	"sort"
	"time"
)

type featureConfig struct {
	name          string
	minimumDF     int
	wordReserve   int
	familyBudgets []int
}

var candidateFeatureConfigs = []featureConfig{
	{name: "all_terms", minimumDF: 1},
	{name: "stable_terms_word_reserve", minimumDF: 2, wordReserve: 1500},
	{name: "balanced_feature_families", minimumDF: 2, familyBudgets: []int{1800, 1500, 1600, 100}},
	{name: "darkweb_error_indicators", minimumDF: 2, familyBudgets: []int{1700, 1500, 1650, 150}},
}

var candidatePositiveWeights = []float64{1, 1.5}

const globalSelectionSourceRecallMinimumPositives = 20

func fitCandidateVectorizer(texts []string, labels []float64, config featureConfig) (*tfidf.Vectorizer, error) {
	if len(config.familyBudgets) == 4 {
		return tfidf.FitWithFamilyBudgets(texts, labels, config.minimumDF, config.familyBudgets[0], config.familyBudgets[1], config.familyBudgets[2], config.familyBudgets[3])
	}
	if config.wordReserve > 0 {
		return tfidf.FitWithWordReserve(texts, labels, config.minimumDF, config.wordReserve)
	}
	return tfidf.FitWithMinDocumentFrequency(texts, labels, config.minimumDF)
}

func candidateWeights(labels []float64, positiveMultiplier float64) []float64 {
	weights := model.BalancedWeights(labels)
	for i, label := range labels {
		if label == 1 {
			weights[i] *= positiveMultiplier
		}
	}
	return weights
}

// Five folds are stratified round-robin over the frozen random development
// order. Vocabulary, IDF and weights are fitted without each held-out fold.
// The separate challenge test is never an input to this command.
func selectThresholdBySource(scores, labels []float64, sources []string, l2 float64) choice {
	best := choice{Threshold: 0, L2: l2, Metrics: measure(scores, labels, 0)}
	sourceFeasible := func(threshold float64) bool {
		groupedScores := map[string][]float64{}
		groupedLabels := map[string][]float64{}
		for i, source := range sources {
			groupedScores[source] = append(groupedScores[source], scores[i])
			groupedLabels[source] = append(groupedLabels[source], labels[i])
		}
		for source := range groupedScores {
			m := measure(groupedScores[source], groupedLabels[source], threshold)
			if m.TP+m.FN >= globalSelectionSourceRecallMinimumPositives && m.Recall < .9 {
				return false
			}
		}
		return true
	}
	best.Feasible = best.Metrics.Precision >= .5 && best.Metrics.Recall >= .9 && sourceFeasible(0)
	for _, threshold := range append(scores, 1) {
		m := measure(scores, labels, threshold)
		candidate := choice{Threshold: threshold, L2: l2, Metrics: m}
		candidate.Feasible = m.Precision >= .5 && m.Recall >= .9 && sourceFeasible(threshold)
		if better(candidate, best) {
			best = candidate
		}
	}
	return best
}

func metricsBySource(scores, labels []float64, sources []string, threshold float64) map[string]metric {
	names := map[string]bool{}
	for _, source := range sources {
		names[source] = true
	}
	ordered := make([]string, 0, len(names))
	for source := range names {
		ordered = append(ordered, source)
	}
	sort.Strings(ordered)
	result := map[string]metric{}
	for _, source := range ordered {
		var sourceScores, sourceLabels []float64
		for i, candidateSource := range sources {
			if candidateSource == source {
				sourceScores = append(sourceScores, scores[i])
				sourceLabels = append(sourceLabels, labels[i])
			}
		}
		result[source] = measure(sourceScores, sourceLabels, threshold)
	}
	return result
}

func averagePrecision(scores, labels []float64) float64 {
	indices := make([]int, len(scores))
	positives := 0
	for i := range scores {
		indices[i] = i
		if labels[i] == 1 {
			positives++
		}
	}
	if positives == 0 {
		return 0
	}
	sort.SliceStable(indices, func(i, j int) bool { return scores[indices[i]] > scores[indices[j]] })
	tp, sum := 0, 0.0
	for rank, index := range indices {
		if labels[index] == 1 {
			tp++
			sum += float64(tp) / float64(rank+1)
		}
	}
	return sum / float64(positives)
}

// sourceSpecificThresholds is an experimental OOF diagnostic. Each supported source
// gets the highest threshold that still protects 90% recall on that source;
// choosing the highest such threshold minimizes false positives for the fixed
// scores. Only sources with enough positive support are serialized; all others
// use the global threshold.
type sourceThresholdDiagnostic struct {
	Threshold       float64 `json:"threshold"`
	Metrics         metric  `json:"metrics"`
	PositiveSupport int     `json:"positive_support"`
	Eligible        bool    `json:"eligible"`
}

// selectRecallConstrainedThreshold maximizes precision while enforcing the
// recall floor. Unlike selectThreshold it does not silently fall back to an
// unconstrained F1 optimum when the precision target is missed.
func selectRecallConstrainedThreshold(scores, labels []float64, minimumRecall float64) choice {
	best := choice{Threshold: 0, Metrics: measure(scores, labels, 0)}
	best.Feasible = best.Metrics.TP+best.Metrics.FN > 0 && best.Metrics.Recall >= minimumRecall
	for _, threshold := range append(append([]float64(nil), scores...), 1) {
		m := measure(scores, labels, threshold)
		candidate := choice{Threshold: threshold, Metrics: m,
			Feasible: m.TP+m.FN > 0 && m.Recall >= minimumRecall}
		if !candidate.Feasible {
			continue
		}
		if !best.Feasible || candidate.Metrics.Precision > best.Metrics.Precision ||
			(candidate.Metrics.Precision == best.Metrics.Precision && candidate.Threshold > best.Threshold) {
			best = candidate
		}
	}
	return best
}

func sourceSpecificThresholds(scores, labels []float64, sources []string, minimumPositives int, fallbackThreshold float64) (map[string]float64, metric, map[string]metric, map[string]sourceThresholdDiagnostic) {
	groupScores := map[string][]float64{}
	groupLabels := map[string][]float64{}
	for i, source := range sources {
		groupScores[source] = append(groupScores[source], scores[i])
		groupLabels[source] = append(groupLabels[source], labels[i])
	}
	thresholds := map[string]float64{}
	bySource := map[string]metric{}
	diagnostics := map[string]sourceThresholdDiagnostic{}
	combined := metric{}
	for source, sourceScores := range groupScores {
		positiveSupport := 0
		for _, label := range groupLabels[source] {
			if label == 1 {
				positiveSupport++
			}
		}
		selected := selectRecallConstrainedThreshold(sourceScores, groupLabels[source], .9)
		eligible := positiveSupport >= minimumPositives && selected.Feasible
		appliedMetrics := selected.Metrics
		if eligible {
			thresholds[source] = selected.Threshold
		} else {
			appliedMetrics = measure(sourceScores, groupLabels[source], fallbackThreshold)
		}
		bySource[source] = appliedMetrics
		diagnostics[source] = sourceThresholdDiagnostic{Threshold: selected.Threshold, Metrics: selected.Metrics, PositiveSupport: positiveSupport, Eligible: eligible}
		combined.TP += appliedMetrics.TP
		combined.FP += appliedMetrics.FP
		combined.FN += appliedMetrics.FN
		combined.TN += appliedMetrics.TN
	}
	if combined.TP+combined.FP > 0 {
		combined.Precision = float64(combined.TP) / float64(combined.TP+combined.FP)
	}
	if combined.TP+combined.FN > 0 {
		combined.Recall = float64(combined.TP) / float64(combined.TP+combined.FN)
	}
	if 2*combined.TP+combined.FP+combined.FN > 0 {
		combined.F1 = float64(2*combined.TP) / float64(2*combined.TP+combined.FP+combined.FN)
	}
	return thresholds, combined, bySource, diagnostics
}

// developmentSimilarityGroups joins provenance groups with exact/near-content
// components. Contradictory components are returned for exclusion: assigning an
// arbitrary label would train and score the same underlying page both ways.
func developmentSimilarityGroups(ids, texts []string, labels []float64, provenance []string) ([]string, map[string]bool, dedup.MergeDiagnostics) {
	groups, exactConflicts := dedup.GroupByNormalizedText(ids, texts, labels)
	merged, nearConflicts, diagnostics := dedup.MergeContentDuplicatesWithDiagnostics(groups)
	conflicting := map[string]bool{}
	for _, conflict := range append(exactConflicts, nearConflicts...) {
		for _, id := range conflict.IDs {
			conflicting[id] = true
		}
	}
	content := map[string]string{}
	for _, component := range merged {
		key := "content:" + component.IDs[0]
		for _, id := range component.IDs {
			content[id] = key
		}
	}
	parent := make([]int, len(ids))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		if parent[i] != i {
			parent[i] = find(parent[i])
		}
		return parent[i]
	}
	seen := map[string]int{}
	for i, id := range ids {
		if conflicting[id] {
			continue
		}
		for _, key := range []string{"provenance:" + provenance[i], content[id]} {
			if key == "" || key == "provenance:" {
				continue
			}
			if j, ok := seen[key]; ok {
				a, b := find(i), find(j)
				if a != b {
					parent[b] = a
				}
			} else {
				seen[key] = i
			}
		}
	}
	result := make([]string, len(ids))
	for i := range ids {
		result[i] = fmt.Sprintf("development-component:%d", find(i))
	}
	return result, conflicting, diagnostics
}

func crossFitCandidate(baseTexts []string, baseLabels []float64, devIDs, devTexts []string, devLabels []float64, devSources, devGroups, devCollectedAt, devSamplingStreams []string, weak int, enableDarkwebHead, includeTargetedSelection bool) (*model.Model, map[string]any, error) {
	if len(devIDs) != len(devLabels) || len(devSources) != len(devLabels) || len(devGroups) != len(devLabels) || len(devCollectedAt) != len(devLabels) || len(devSamplingStreams) != len(devLabels) {
		return nil, nil, fmt.Errorf("development metadata count mismatch")
	}
	baseTexts, baseLabels, excludedExactOverlap, excludedNearOverlap := excludeDevelopmentOverlap(baseTexts, baseLabels, devTexts)
	effectiveGroups, conflictingIDs, developmentDuplicateDiagnostics := developmentSimilarityGroups(devIDs, devTexts, devLabels, devGroups)
	conflictingDocuments := len(conflictingIDs)
	if conflictingDocuments > 0 {
		keep := make([]int, 0, len(devIDs)-conflictingDocuments)
		for i, id := range devIDs {
			if !conflictingIDs[id] {
				keep = append(keep, i)
			}
		}
		devIDs = selectStringValues(devIDs, keep)
		devTexts = selectStringValues(devTexts, keep)
		devLabels = selectFloatValues(devLabels, keep)
		devSources = selectStringValues(devSources, keep)
		devGroups = selectStringValues(effectiveGroups, keep)
		devCollectedAt = selectStringValues(devCollectedAt, keep)
		devSamplingStreams = selectStringValues(devSamplingStreams, keep)
	} else {
		devGroups = effectiveGroups
	}
	folds := developmentFolds(devLabels, devSources, devGroups)
	l2s := []float64{.001, .01, .1, .5}
	type candidate struct {
		config             featureConfig
		l2                 float64
		positiveMultiplier float64
	}
	var candidates []candidate
	for _, config := range candidateFeatureConfigs {
		for _, positiveMultiplier := range candidatePositiveWeights {
			for _, l2 := range l2s {
				candidates = append(candidates, candidate{config: config, l2: l2, positiveMultiplier: positiveMultiplier})
			}
		}
	}
	scores := make([][]float64, len(candidates))
	for i := range scores {
		scores[i] = make([]float64, len(devLabels))
	}
	// The shared-head OOF pass is mandatory. enableDarkwebHead only controls
	// the optional source-specific head built below; coupling it to this loop
	// silently left every shared OOF score at zero when that experiment was off.
	for fold := 0; fold < 5; fold++ {
		texts := append([]string(nil), baseTexts...)
		labels := append([]float64(nil), baseLabels...)
		for i, t := range devTexts {
			if folds[i] != fold {
				texts = append(texts, t)
				labels = append(labels, devLabels[i])
			}
		}
		for _, config := range candidateFeatureConfigs {
			v, err := fitCandidateVectorizer(texts, labels, config)
			if err != nil {
				return nil, nil, err
			}
			X := make([][]float64, len(texts))
			for i, t := range texts {
				X[i] = v.Transform(t)
			}
			held := map[int][]float64{}
			for i, t := range devTexts {
				if folds[i] == fold {
					held[i] = v.Transform(t)
				}
			}
			for k, candidate := range candidates {
				if candidate.config.name != config.name {
					continue
				}
				lr := model.TrainWeighted(X, labels, candidateWeights(labels, candidate.positiveMultiplier), 4000, 2, candidate.l2)
				for i, x := range held {
					scores[k][i] = lr.Predict(x)
				}
			}
		}
		fmt.Fprintf(os.Stderr, "Finished development fold %d/5; independent test not read\n", fold+1)
	}
	best := choice{Metrics: metric{F1: math.Inf(-1)}}
	bestIndex := 0
	trials := []choice{}
	selectionIndices := developmentSelectionIndices(devSamplingStreams, includeTargetedSelection)
	selectionLabels := selectFloatValues(devLabels, selectionIndices)
	selectionSources := selectStringValues(devSources, selectionIndices)
	for i, candidate := range candidates {
		c := selectThresholdBySource(selectFloatValues(scores[i], selectionIndices), selectionLabels, selectionSources, candidate.l2)
		c.FeatureConfig = candidate.config.name
		c.PositiveWeight = candidate.positiveMultiplier
		trials = append(trials, c)
		if i == 0 || better(c, best) {
			best = c
			bestIndex = i
		}
	}
	const darkwebHeadL2 = .1
	hybridScores := append([]float64(nil), scores[bestIndex]...)
	for fold := 0; enableDarkwebHead && fold < 5; fold++ {
		vectorizerTexts := append([]string(nil), baseTexts...)
		vectorizerLabels := append([]float64(nil), baseLabels...)
		for i, text := range devTexts {
			if folds[i] != fold {
				vectorizerTexts = append(vectorizerTexts, text)
				vectorizerLabels = append(vectorizerLabels, devLabels[i])
			}
		}
		vFold, fitErr := fitCandidateVectorizer(vectorizerTexts, vectorizerLabels, candidates[bestIndex].config)
		if fitErr != nil {
			return nil, nil, fitErr
		}
		var headX [][]float64
		var headLabels []float64
		for i, text := range devTexts {
			if folds[i] != fold && devSources[i] == "darkweb_page" {
				headX = append(headX, vFold.Transform(text))
				headLabels = append(headLabels, devLabels[i])
			}
		}
		head := model.TrainWeighted(headX, headLabels, model.BalancedWeights(headLabels), 4000, 2, darkwebHeadL2)
		for i, text := range devTexts {
			if folds[i] == fold && devSources[i] == "darkweb_page" {
				hybridScores[i] = head.Predict(vFold.Transform(text))
			}
		}
	}
	texts := append(append([]string(nil), baseTexts...), devTexts...)
	labels := append(append([]float64(nil), baseLabels...), devLabels...)
	v, err := fitCandidateVectorizer(texts, labels, candidates[bestIndex].config)
	if err != nil {
		return nil, nil, err
	}
	X := make([][]float64, len(texts))
	for i, t := range texts {
		X[i] = v.Transform(t)
	}
	lr := model.TrainWeighted(X, labels, candidateWeights(labels, best.PositiveWeight), 4000, 2, best.L2)
	var sourceHeads map[string]model.SourceHead
	if enableDarkwebHead {
		var darkwebX [][]float64
		var darkwebLabels []float64
		for i, text := range devTexts {
			if devSources[i] == "darkweb_page" {
				darkwebX = append(darkwebX, v.Transform(text))
				darkwebLabels = append(darkwebLabels, devLabels[i])
			}
		}
		darkwebHead := model.TrainWeighted(darkwebX, darkwebLabels, model.BalancedWeights(darkwebLabels), 4000, 2, darkwebHeadL2)
		sourceHeads = map[string]model.SourceHead{"darkweb_page": {Weights: darkwebHead.Weights, Bias: darkwebHead.Bias}}
	}
	// Below 100 representative positives, a source-specific threshold is too
	// sensitive to a handful of examples and must fall back to the global one.
	const sourceMinimumPositives = 100
	selectionScores := selectFloatValues(hybridScores, selectionIndices)
	// The global calibrator belongs to the shared head. Fitting it on hybrid
	// scores would leak the darkweb head's different scale into every other
	// source and the unknown-source fallback.
	sharedSelectionScores := selectFloatValues(scores[bestIndex], selectionIndices)
	sourceThresholds, sourceMetrics, sourceMetricsBySource, sourceDiagnostics := sourceSpecificThresholds(selectionScores, selectionLabels, selectionSources, sourceMinimumPositives, best.Threshold)
	effectiveBySourceObjectives := map[string]bool{}
	allSourcesMeetGlobalTargets := true
	for source, metrics := range sourceMetricsBySource {
		met := metrics.Precision >= .5 && metrics.Recall >= .9
		effectiveBySourceObjectives[source] = met
		if !met {
			allSourcesMeetGlobalTargets = false
		}
	}
	effectiveGlobalObjectiveMet := sourceMetrics.Precision >= .5 && sourceMetrics.Recall >= .9
	var darkwebOOFScores, darkwebOOFLabels []float64
	for i, source := range devSources {
		if source == "darkweb_page" && devSamplingStreams[i] != "targeted_development_enrichment" {
			darkwebOOFScores = append(darkwebOOFScores, hybridScores[i])
			darkwebOOFLabels = append(darkwebOOFLabels, devLabels[i])
		}
	}
	selectionAuthority := "representative-stream grouped OOF only; targeted enrichment excluded"
	selectionStream := "representative_development_only"
	if includeTargetedSelection {
		selectionAuthority = "targeted-development grouped OOF only; exploratory and not a deployment qualification"
		selectionStream = "targeted_development_enrichment_OOF_exploratory"
	}
	m := &model.Model{Vocabulary: v.Vocabulary, IDF: v.IDF, Weights: lr.Weights, Bias: lr.Bias, Threshold: best.Threshold,
		SourceThresholds: sourceThresholds,
		SourceHeads:      sourceHeads,
		Calibration:      model.FitPlattScaling(sharedSelectionScores, selectionLabels, 2000, .1), Version: "provisional-oof-advisories-" + time.Now().UTC().Format("20060102T150405Z"),
		Validation: &model.ValidationSummary{Documents: len(selectionLabels), Precision: best.Metrics.Precision, Recall: best.Metrics.Recall, F1: best.Metrics.F1, TargetPrecision: .5, MinimumRecall: .9, ObjectiveMet: best.Feasible, EffectivePrecision: sourceMetrics.Precision, EffectiveRecall: sourceMetrics.Recall, EffectiveF1: sourceMetrics.F1, PromotionEligible: boolPointer(effectiveGlobalObjectiveMet && allSourcesMeetGlobalTargets), SelectionRule: selectionAuthority + "; no independent challenge read", LabelAuthority: "AI development consensus; exploratory OOF estimates, not independent test"}}
	if enableDarkwebHead {
		head := m.SourceHeads["darkweb_page"]
		head.Calibration = model.FitPlattScaling(darkwebOOFScores, darkwebOOFLabels, 2000, .1)
		m.SourceHeads["darkweb_page"] = head
	}
	predictions := make([]map[string]any, len(devLabels))
	for i := range devLabels {
		predictions[i] = map[string]any{"id": devIDs[i], "source": devSources[i], "group": devGroups[i], "collected_at": devCollectedAt[i], "sampling_stream": devSamplingStreams[i], "fold": folds[i], "label": devLabels[i], "score": hybridScores[i]}
	}
	return m, map[string]any{"selected": best, "selected_oof_predictions": predictions, "oof_average_precision": averagePrecision(selectionScores, selectionLabels), "selected_metrics_by_source": metricsBySource(selectionScores, selectionLabels, selectionSources, best.Threshold), "experimental_source_thresholds": sourceThresholds, "experimental_source_threshold_metrics": sourceMetrics, "experimental_source_threshold_metrics_by_source": sourceMetricsBySource, "selection_global_objective_met": best.Feasible, "effective_policy_global_objective_met": effectiveGlobalObjectiveMet, "effective_policy_all_sources_meet_global_targets": allSourcesMeetGlobalTargets, "effective_policy_objective_by_source": effectiveBySourceObjectives, "source_threshold_diagnostics": sourceDiagnostics, "source_recall_constraint": .9, "source_constraint_minimum_positives": sourceMinimumPositives, "source_threshold_minimum_positives": sourceMinimumPositives, "global_selection_source_recall_minimum_positives": globalSelectionSourceRecallMinimumPositives, "source_head_experiment": map[string]any{"enabled": enableDarkwebHead, "source": "darkweb_page", "l2": darkwebHeadL2, "training": "source-only head, common vectorizer, fit inside each held-out fold", "hyperparameter_selected_on_oof": false}, "trials": trials, "training_documents": len(texts), "training_documents_excluded_for_exact_development_overlap": excludedExactOverlap, "training_documents_excluded_for_near_development_overlap": excludedNearOverlap, "development_documents_excluded_for_conflicting_duplicate_labels": conflictingDocuments, "development_near_duplicate_component_diagnostics": developmentDuplicateDiagnostics, "weak_advisories_input": weak, "AI_development_documents": len(devTexts), "selection_documents": len(selectionLabels), "targeted_enrichment_documents": len(devLabels) - len(selectionLabels), "calibration_documents": len(selectionLabels), "calibration_sampling_stream": selectionStream, "targeted_stream_included_for_selection": includeTargetedSelection, "cross_validation_folds": 5, "cross_validation_grouped": true, "cross_validation_group_field": "provenance_and_near_duplicate_connected_component", "collected_at_present": nonEmptyCount(devCollectedAt), "vocabulary_fitted_inside_each_fold": true, "candidate_family": "shared feature configuration x positive weight x L2; optional pre-registered fixed darkweb source head recorded separately", "threshold_authority": selectionAuthority, "test_read": false, "production_promotion": false}, nil
}

func developmentSelectionIndices(streams []string, includeTargeted bool) []int {
	indexes := make([]int, 0, len(streams))
	for i, stream := range streams {
		if includeTargeted || stream != "targeted_development_enrichment" {
			indexes = append(indexes, i)
		}
	}
	return indexes
}

func boolPointer(value bool) *bool { return &value }

func selectFloatValues(values []float64, indices []int) []float64 {
	selected := make([]float64, len(indices))
	for i, index := range indices {
		selected[i] = values[index]
	}
	return selected
}

func selectStringValues(values []string, indices []int) []string {
	selected := make([]string, len(indices))
	for i, index := range indices {
		selected[i] = values[index]
	}
	return selected
}

// excludeDevelopmentOverlap prevents a development document from appearing in
// the fixed base training set of every OOF fold. Source files remain untouched.
func excludeDevelopmentOverlap(baseTexts []string, baseLabels []float64, devTexts []string) ([]string, []float64, int, int) {
	development := make(map[string]struct{}, len(devTexts))
	developmentPrints := make([]overlap.Print, 0, len(devTexts))
	for _, candidate := range devTexts {
		if normalized := dedup.Normalize(candidate); normalized != "" {
			development[normalized] = struct{}{}
		}
		developmentPrints = append(developmentPrints, overlap.Fingerprint(candidate))
	}
	texts := make([]string, 0, len(baseTexts))
	labels := make([]float64, 0, len(baseLabels))
	excludedExact, excludedNear := 0, 0
	for i, candidate := range baseTexts {
		if _, overlaps := development[dedup.Normalize(candidate)]; overlaps {
			excludedExact++
			continue
		}
		candidatePrint := overlap.Fingerprint(candidate)
		near := false
		for _, developmentPrint := range developmentPrints {
			if overlap.Similar(candidatePrint, developmentPrint) {
				near = true
				break
			}
		}
		if near {
			excludedNear++
			continue
		}
		texts = append(texts, candidate)
		labels = append(labels, baseLabels[i])
	}
	return texts, labels, excludedExact, excludedNear
}

func nonEmptyCount(values []string) int {
	n := 0
	for _, value := range values {
		if value != "" {
			n++
		}
	}
	return n
}

// developmentFolds keeps every provenance group in one fold. Groups are
// assigned greedily to the fold with the smallest count for their dominant
// source/label stratum, preserving stratification without host leakage.
func developmentFolds(labels []float64, sources, groups []string) []int {
	folds := make([]int, len(labels))
	type bucket struct {
		name    string
		indices []int
		strata  map[string]int
	}
	byGroup := map[string]*bucket{}
	order := []string{}
	for i, label := range labels {
		source := ""
		if i < len(sources) {
			source = sources[i]
		}
		group := ""
		if i < len(groups) {
			group = groups[i]
		}
		if group == "" {
			group = fmt.Sprintf("document:%d", i)
		}
		if byGroup[group] == nil {
			byGroup[group] = &bucket{name: group, strata: map[string]int{}}
			order = append(order, group)
		}
		byGroup[group].indices = append(byGroup[group].indices, i)
		byGroup[group].strata[fmt.Sprintf("%s\x00%d", source, int(label))]++
	}
	sort.SliceStable(order, func(i, j int) bool {
		a, b := byGroup[order[i]], byGroup[order[j]]
		if len(a.indices) != len(b.indices) {
			return len(a.indices) > len(b.indices)
		}
		return a.name < b.name
	})
	counts := map[string][5]int{}
	totals := map[string]int{}
	for _, b := range byGroup {
		for stratum, n := range b.strata {
			totals[stratum] += n
		}
	}
	for _, name := range order {
		b := byGroup[name]
		chosen := 0
		bestCost := math.Inf(1)
		for fold := 0; fold < 5; fold++ {
			cost := 0.0
			for stratum, added := range b.strata {
				byFold := counts[stratum]
				target := float64(totals[stratum]) / 5
				for candidateFold := 0; candidateFold < 5; candidateFold++ {
					value := byFold[candidateFold]
					if candidateFold == fold {
						value += added
					}
					delta := float64(value) - target
					cost += delta * delta / math.Max(target, 1)
				}
			}
			if cost < bestCost {
				bestCost, chosen = cost, fold
			}
		}
		for _, index := range b.indices {
			folds[index] = chosen
		}
		for stratum, added := range b.strata {
			byFold := counts[stratum]
			byFold[chosen] += added
			counts[stratum] = byFold
		}
	}
	return folds
}
