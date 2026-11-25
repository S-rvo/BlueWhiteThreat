package main

import (
	"fmt"
	"testing"
)

func TestConstraintTakesPrecedenceOverF1(t *testing.T) {
	valid := choice{Feasible: true, Metrics: metric{Precision: .55, Recall: .95, F1: .69}}
	invalid := choice{Feasible: false, Metrics: metric{Precision: .95, Recall: .85, F1: .90}}
	if !better(valid, invalid) || better(invalid, valid) {
		t.Fatal("infeasible candidate overrode recall requirement")
	}
}
func TestThresholdHandlesNoPositivesWithoutClaimingSuccess(t *testing.T) {
	c := selectThreshold([]float64{.2, .4}, []float64{0, 0}, .1)
	if c.Feasible || c.Metrics.Recall != 0 || c.Threshold != 1 {
		t.Fatal("empty positive class claimed success")
	}
}
func TestSelectionUsesExactThresholdBoundary(t *testing.T) {
	c := selectThreshold([]float64{.7, .6, .4, .1}, []float64{1, 1, 0, 0}, .1)
	if !c.Feasible || c.Threshold != .6 || c.Metrics.Precision != 1 || c.Metrics.Recall != 1 {
		t.Fatalf("incorrect threshold: %+v", c)
	}
}

func TestAveragePrecisionPerfectRanking(t *testing.T) {
	if got := averagePrecision([]float64{.9, .8, .2, .1}, []float64{1, 1, 0, 0}); got != 1 {
		t.Fatalf("average precision=%v", got)
	}
}

func TestDevelopmentSelectionExcludesTargetedByDefault(t *testing.T) {
	streams := []string{"representative", "targeted_development_enrichment", "representative"}
	got := developmentSelectionIndices(streams, false)
	if fmt.Sprint(got) != "[0 2]" {
		t.Fatalf("default selection indexes=%v, want [0 2]", got)
	}
	got = developmentSelectionIndices(streams, true)
	if fmt.Sprint(got) != "[0 1 2]" {
		t.Fatalf("explicit exploratory selection indexes=%v, want [0 1 2]", got)
	}
}

func TestSourceAwareThresholdProtectsRecallInEverySupportedSource(t *testing.T) {
	var scores, labels []float64
	var sources []string
	for i := 0; i < 20; i++ {
		score := .9
		if i < 3 {
			score = .2
		}
		scores = append(scores, score)
		labels = append(labels, 1)
		sources = append(sources, "ransomware.live")
	}
	for i := 0; i < 20; i++ {
		scores = append(scores, .9)
		labels = append(labels, 1)
		sources = append(sources, "github-advisories")
	}
	for i := 0; i < 60; i++ {
		scores = append(scores, .1)
		labels = append(labels, 0)
		sources = append(sources, "github-advisories")
	}
	c := selectThresholdBySource(scores, labels, sources, .1)
	if !c.Feasible || c.Threshold != .2 {
		t.Fatalf("source recall constraint did not lower threshold: %+v", c)
	}
	bySource := metricsBySource(scores, labels, sources, c.Threshold)
	if bySource["ransomware.live"].Recall < .9 {
		t.Fatalf("ransomware recall not protected: %+v", bySource)
	}
}

func TestSourceSpecificThresholdsReduceCrossSourceFalsePositives(t *testing.T) {
	var scores, labels []float64
	var sources []string
	for _, source := range []string{"a", "b"} {
		for i := 0; i < 20; i++ {
			scores = append(scores, .8)
			labels = append(labels, 1)
			sources = append(sources, source)
		}
		for i := 0; i < 20; i++ {
			scores = append(scores, .4)
			labels = append(labels, 0)
			sources = append(sources, source)
		}
	}
	thresholds, combined, bySource, diagnostics := sourceSpecificThresholds(scores, labels, sources, 20, .5)
	if thresholds["a"] != .8 || thresholds["b"] != .8 {
		t.Fatalf("unexpected thresholds: %#v", thresholds)
	}
	if combined.FP != 0 || combined.Recall != 1 || bySource["a"].Recall != 1 || bySource["b"].Recall != 1 {
		t.Fatalf("unexpected metrics: combined=%+v by-source=%+v", combined, bySource)
	}
	if !diagnostics["a"].Eligible || !diagnostics["b"].Eligible {
		t.Fatal("supported sources not eligible")
	}
}

func TestSourceSpecificThresholdOmittedBelowMinimumSupport(t *testing.T) {
	scores := []float64{.9, .8, .7, .1}
	labels := []float64{1, 1, 0, 0}
	sources := []string{"darkweb_page", "darkweb_page", "darkweb_page", "darkweb_page"}
	thresholds, combined, bySource, diagnostics := sourceSpecificThresholds(scores, labels, sources, 20, .95)
	if _, ok := thresholds["darkweb_page"]; ok {
		t.Fatal("unstable source threshold was serialized")
	}
	if diagnostics["darkweb_page"].Eligible || diagnostics["darkweb_page"].PositiveSupport != 2 {
		t.Fatalf("bad diagnostic: %+v", diagnostics)
	}
	if bySource["darkweb_page"].Recall != 0 || combined.Recall != 0 {
		t.Fatalf("unsupported source did not use global fallback: combined=%+v by-source=%+v", combined, bySource)
	}
}

func TestDevelopmentFoldsStratifyEverySourceAndLabel(t *testing.T) {
	var labels []float64
	var sources []string
	for _, source := range []string{"a", "b"} {
		for _, label := range []float64{0, 1} {
			for i := 0; i < 10; i++ {
				labels = append(labels, label)
				sources = append(sources, source)
			}
		}
	}
	folds := developmentFolds(labels, sources, make([]string, len(labels)))
	counts := map[string][5]int{}
	for i, fold := range folds {
		key := fmt.Sprintf("%s/%d", sources[i], int(labels[i]))
		value := counts[key]
		value[fold]++
		counts[key] = value
	}
	for key, byFold := range counts {
		for fold, count := range byFold {
			if count != 2 {
				t.Fatalf("%s fold %d has %d examples, want 2", key, fold, count)
			}
		}
	}
}

func TestDevelopmentFoldsPreserveMinorityRepresentation(t *testing.T) {
	labels := make([]float64, 184)
	for i := 0; i < 19; i++ {
		labels[i] = 1
	}
	folds := developmentFolds(labels, make([]string, len(labels)), make([]string, len(labels)))
	counts := [5][2]int{}
	for i, fold := range folds {
		if fold < 0 || fold >= 5 {
			t.Fatal("invalid fold")
		}
		counts[fold][int(labels[i])]++
	}
	for _, c := range counts {
		if c[1] < 3 || c[1] > 4 || c[0] != 33 {
			t.Fatalf("minority or majority unbalanced: %v", counts)
		}
	}
}

func TestDevelopmentFoldsNeverSplitGroup(t *testing.T) {
	labels := []float64{1, 1, 1, 0, 0, 0}
	sources := []string{"a", "a", "a", "a", "a", "a"}
	groups := []string{"host:x", "host:x", "host:x", "host:y", "host:y", "host:z"}
	folds := developmentFolds(labels, sources, groups)
	seen := map[string]int{}
	for i, group := range groups {
		if fold, ok := seen[group]; ok && fold != folds[i] {
			t.Fatalf("group %s split across folds", group)
		}
		seen[group] = folds[i]
	}
}

func TestDevelopmentSimilarityGroupsJoinsClonesAndExcludesConflicts(t *testing.T) {
	ids := []string{"a", "b", "c", "d"}
	texts := []string{
		"one two three four five six seven eight nine ten eleven twelve thirteen fourteen fifteen sixteen seventeen eighteen nineteen twenty",
		"one two three four five six seven eight nine ten eleven twelve thirteen fourteen fifteen sixteen seventeen eighteen nineteen twenty extra",
		"contradictory page", "CONTRADICTORY   PAGE",
	}
	labels := []float64{1, 1, 0, 1}
	groups, conflicts, _ := developmentSimilarityGroups(ids, texts, labels, []string{"host-a", "host-b", "host-c", "host-d"})
	if groups[0] != groups[1] {
		t.Fatalf("near-identical pages must share a fold group: %v", groups)
	}
	if !conflicts["c"] || !conflicts["d"] {
		t.Fatalf("exact contradictory pages must be excluded: %v", conflicts)
	}
}

func TestCandidateWeightsIncreaseOnlyPositiveCost(t *testing.T) {
	labels := []float64{0, 0, 1, 1}
	base := candidateWeights(labels, 1)
	weighted := candidateWeights(labels, 1.5)
	for i, label := range labels {
		if label == 0 && weighted[i] != base[i] {
			t.Fatalf("negative weight changed at %d", i)
		}
		if label == 1 && weighted[i] != 1.5*base[i] {
			t.Fatalf("positive weight was not increased at %d", i)
		}
	}
}

func TestCandidateFamilyIncludesStableVocabularyAndRecallWeighting(t *testing.T) {
	if len(candidateFeatureConfigs) < 2 || len(candidatePositiveWeights) < 2 {
		t.Fatal("candidate family does not vary features and positive cost")
	}
	if candidateFeatureConfigs[1].minimumDF < 2 || candidateFeatureConfigs[1].wordReserve == 0 {
		t.Fatal("stable feature configuration is missing its filters")
	}
}

func TestExcludeDevelopmentOverlapUsesNormalizedContent(t *testing.T) {
	texts, labels, excluded, near := excludeDevelopmentOverlap(
		[]string{"Unique base", "Shared   advisory", "another"},
		[]float64{0, 1, 0},
		[]string{"shared advisory"},
	)
	if excluded != 1 {
		t.Fatalf("excluded=%d, want 1", excluded)
	}
	if near != 0 {
		t.Fatalf("near=%d, want 0", near)
	}
	if len(texts) != 2 || len(labels) != 2 || texts[0] != "Unique base" || texts[1] != "another" {
		t.Fatalf("unexpected filtered data: %#v %#v", texts, labels)
	}
}

func TestExcludeDevelopmentOverlapRejectsNearDuplicate(t *testing.T) {
	base := "one two three four five six seven eight nine ten eleven twelve thirteen fourteen fifteen sixteen seventeen eighteen nineteen twenty twentyone twentytwo twentythree twentyfour twentyfive"
	development := base + " extra"
	texts, labels, exact, near := excludeDevelopmentOverlap([]string{base, "short unique"}, []float64{1, 0}, []string{development})
	if exact != 0 || near != 1 || len(texts) != 1 || len(labels) != 1 || texts[0] != "short unique" {
		t.Fatalf("unexpected exclusion: exact=%d near=%d texts=%#v labels=%#v", exact, near, texts, labels)
	}
}
