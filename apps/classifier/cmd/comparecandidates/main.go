// apps/classifier/cmd/comparecandidates/main.go
//
// Unified comparison of three training-label strategies, same TF-IDF +
// logistic-regression architecture, evaluated on the same fixed
// human-annotated validation set:
//
//  1. "old"   - OLD heuristic (internal/heuristiccompare.OldLabelByKeywords)
//     fills labels for non-reviewed documents.
//  2. "new"   - current shipping heuristic (internal/bootstrap.LabelByKeywords,
//     MinKeywordHits=3) fills labels for non-reviewed documents. The
//     minMatches=1 sweep result from cmd/evalheuristics stays experimental
//     and is NOT substituted here.
//  3. "human" - trained only on documents with a human-reviewed label; no
//     heuristic label is ever used.
//
// Earlier tools (cmd/compareheuristics, cmd/evalheuristics) evaluated these
// candidates on two different targets: the heuristic-independent TEST
// partition, and the validation split, respectively. That made them
// impossible to compare head-to-head. This tool fixes that by scoring every
// candidate on the exact same validation set, which is also the set
// parameters (threshold, calibration) are picked on per the standing rule.
//
// The frozen TEST partition (107 documents with ground truth) is printed
// too, but explicitly labeled DEV BENCHMARK: its aggregate metrics have
// already been consulted by earlier tool runs, so it is no longer eligible
// as blind final confirmation. Final confirmation must come from
// cmd/confirmfinal against newly annotated documents outside the frozen
// partition entirely.
package main

import (
	"log"
	"os"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/bootstrap"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/evalmetrics"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/frozen"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/heuristiccompare"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/model"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/review"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/tfidf"
	"github.com/joho/godotenv"
)

const (
	trainRatio              = 0.7
	valRatio                = 0.15
	splitSeed               = 42
	epochs                  = 4000
	learningRate            = 2.0
	l2                      = 0.5
	targetPrecision         = 0.99
	minRecall               = 0.3
	calibrationEpochs       = 2000
	calibrationLearningRate = 0.1

	// squaredHingeEpochs/squaredHingeL2: model.TrainSquaredHinge's step is
	// fixed at 0.1 and is not tuned for comparecandidates' logistic
	// learningRate/l2 constants. A recall sweep on validation only (positive
	// class weight multipliers 1-8x, l2 grid 0.0001-0.0008, epochs up to
	// 30000) found positive-class overweighting hurts recall, but more
	// epochs plus reserving TF-IDF vocabulary slots for plain words (pushing
	// out some character n-grams via tfidf.FitWithWordReserve) raises
	// validation recall from 0.558 (epochs=5000, default vocab) to 0.621
	// (f1 0.716->0.766) at the same precision=1.000; a fine-grained reserve
	// sweep around the initial 4500 estimate found 4400 as the local
	// optimum (4300/4350/4450-4700 all tie or trail it); combining epochs
	// beyond 20000 with the word reserve adds nothing further.
	squaredHingeEpochs      = 20000
	squaredHingeL2          = 0.0008
	squaredHingeWordReserve = 4400

	// cappedHingeEpochs: unchanged from before the squared-hinge sweep above;
	// the wordReserve/epochs=20000 combination was only validated for
	// squared-hinge, not capped-hinge, so capped-hinge keeps its prior
	// epochs=5000/default-vocab configuration until it gets its own sweep.
	cappedHingeEpochs = 5000
)

type candidateResult struct {
	name      string
	threshold float64
	val       metricSet
	dev       metricSet // frozen TEST partition, dev-benchmark only
}

type metricSet struct {
	n                               int
	accuracy, precision, recall, f1 float64
	tp, fp, tn, fn                  int
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env found or unreadable, continuing with existing environment.")
	}

	trainingDataPath := getEnvOrDefault("TRAINING_DATA", "training_data.csv")
	keywordsPath := getEnvOrDefault("SEED_KEYWORDS_PATH", "data/seed_keywords.json")
	reviewedPath := getEnvOrDefault("REVIEWED_LABELS_PATH", "data/reviewed_labels.csv")

	ids, texts, currentLabels, err := evalmetrics.ReadLabeledCSV(trainingDataPath)
	if err != nil {
		log.Fatalf("failed to read %s: %v", trainingDataPath, err)
	}
	log.Printf("Loaded %d documents from %s (frozen text corpus)", len(ids), trainingDataPath)

	keywords, err := bootstrap.LoadKeywords(keywordsPath)
	if err != nil {
		log.Fatalf("failed to load seed keywords: %v", err)
	}

	reviewedLabels, err := review.LoadReviewed(reviewedPath)
	if err != nil {
		log.Fatalf("failed to load reviewed labels: %v", err)
	}
	reviewed := map[string]int{}
	for id, l := range reviewedLabels {
		reviewed[id] = l.Label
	}
	log.Printf("Loaded %d human-reviewed ground-truth labels from %s", len(reviewed), reviewedPath)

	idText := make(map[string]string, len(ids))
	for i, id := range ids {
		idText[id] = texts[i]
	}

	oldLabels := make(map[string]float64, len(ids))
	newLabels := make(map[string]float64, len(ids))
	for i, id := range ids {
		if human, ok := reviewed[id]; ok {
			oldLabels[id] = float64(human)
			newLabels[id] = float64(human)
			continue
		}
		oldLabels[id] = float64(heuristiccompare.OldLabelByKeywords(texts[i], keywords))
		newLabels[id] = float64(bootstrap.LabelByKeywords(texts[i], keywords))
	}

	textGroups := frozen.GroupByTextOnly(ids, texts)
	refLabels := make(map[string]float64, len(ids))
	for i, id := range ids {
		refLabels[id] = currentLabels[i]
	}
	trainIDs, valIDs, testIDs := frozen.Partition(textGroups, refLabels, splitSeed, trainRatio, valRatio)
	log.Printf("Frozen partition: %d train ids, %d val ids, %d test ids", len(trainIDs), len(valIDs), len(testIDs))

	// The fixed human-annotated validation set: every candidate is scored
	// here, and ONLY here, for the purpose of choosing a winner.
	var valTexts []string
	var valGroundTruth []float64
	for id := range valIDs {
		if human, ok := reviewed[id]; ok {
			valTexts = append(valTexts, idText[id])
			valGroundTruth = append(valGroundTruth, float64(human))
		}
	}
	log.Printf("Fixed human validation set: %d/%d frozen val ids have human ground truth", len(valGroundTruth), len(valIDs))
	if len(valGroundTruth) == 0 {
		log.Fatal("no human-reviewed ground truth available in the frozen validation partition")
	}

	// DEV BENCHMARK ONLY: the frozen TEST partition. Its aggregate metrics
	// have already been printed by cmd/compareheuristics in an earlier
	// session, so it is disqualified as a blind final check. Kept here
	// purely for continuity with earlier runs, never for candidate
	// selection.
	var devTexts []string
	var devGroundTruth []float64
	for id := range testIDs {
		if human, ok := reviewed[id]; ok {
			devTexts = append(devTexts, idText[id])
			devGroundTruth = append(devGroundTruth, float64(human))
		}
	}
	log.Printf("DEV BENCHMARK (already consulted, NOT eligible as final confirmation): %d/%d frozen test ids have human ground truth",
		len(devGroundTruth), len(testIDs))

	var results []candidateResult

	results = append(results, runHeuristicCandidate("old", oldLabels, textGroups, trainIDs,
		idText, valTexts, valGroundTruth, devTexts, devGroundTruth, "model_old.json"))
	results = append(results, runHeuristicCandidate("new", newLabels, textGroups, trainIDs,
		idText, valTexts, valGroundTruth, devTexts, devGroundTruth, "model_new.json"))
	results = append(results, runHumanOnlyCandidate(textGroups, trainIDs, reviewed,
		idText, valTexts, valGroundTruth, devTexts, devGroundTruth, "model_human.json"))
	humanSquaredHinge, err := runHumanOnlyMarginLoss("human-squared-hinge", false, squaredHingeWordReserve, squaredHingeEpochs, textGroups, trainIDs, reviewed,
		idText, valTexts, valGroundTruth, devTexts, devGroundTruth, "model_human_squared_hinge.json")
	if err != nil {
		log.Fatalf("candidate %q: squared-hinge training failed: %v", "human-squared-hinge", err)
	}
	results = append(results, humanSquaredHinge)
	humanCappedHinge, err := runHumanOnlyMarginLoss("human-capped-hinge", true, 0, cappedHingeEpochs, textGroups, trainIDs, reviewed,
		idText, valTexts, valGroundTruth, devTexts, devGroundTruth, "model_human_capped_hinge.json")
	if err != nil {
		log.Fatalf("candidate %q: capped-hinge training failed: %v", "human-capped-hinge", err)
	}
	results = append(results, humanCappedHinge)

	log.Println("=== THREE-WAY COMPARISON — fixed human validation set (selection happens HERE, nowhere else) ===")
	best := -1
	for i, r := range results {
		log.Printf("  %-6s  val(n=%d) accuracy=%.3f precision=%.3f recall=%.3f f1=%.3f  |  dev-benchmark(n=%d) accuracy=%.3f precision=%.3f recall=%.3f f1=%.3f",
			r.name, r.val.n, r.val.accuracy, r.val.precision, r.val.recall, r.val.f1,
			r.dev.n, r.dev.accuracy, r.dev.precision, r.dev.recall, r.dev.f1)
		if best == -1 || r.val.f1 > results[best].val.f1 {
			best = i
		}
	}
	log.Printf("=== Candidate selected on validation: %q (val f1=%.3f) ===", results[best].name, results[best].val.f1)
	log.Println("This selection is provisional until confirmed ONCE against newly annotated documents outside the frozen partition (see cmd/confirmfinal).")
	log.Println("The dev-benchmark numbers above are for continuity only — they must not be used to break ties or justify the choice.")
}

// runHeuristicCandidate trains on a heuristic-plus-human label map, dropping
// any frozen group inconsistent under that map, then scores on the shared
// validation set and the dev-benchmark test set.
func runHeuristicCandidate(name string, variantLabels map[string]float64, textGroups []frozen.TextGroup, trainIDs map[string]bool,
	idText map[string]string, valTexts []string, valGroundTruth []float64, devTexts []string, devGroundTruth []float64, modelPath string) candidateResult {

	var trainTexts []string
	var trainLabels []float64
	for _, g := range textGroups {
		consistent := true
		label := variantLabels[g.IDs[0]]
		for _, id := range g.IDs[1:] {
			if variantLabels[id] != label {
				consistent = false
				break
			}
		}
		if !consistent {
			continue
		}
		for _, id := range g.IDs {
			if trainIDs[id] {
				trainTexts = append(trainTexts, idText[id])
				trainLabels = append(trainLabels, label)
			}
		}
	}
	log.Printf("candidate %q: training on %d examples", name, len(trainTexts))

	vectorizer := tfidf.Fit(trainTexts, trainLabels)
	X := make([][]float64, len(trainTexts))
	for i, t := range trainTexts {
		X[i] = vectorizer.Transform(t)
	}
	weights := model.BalancedWeights(trainLabels)
	lr := model.TrainWeighted(X, trainLabels, weights, epochs, learningRate, l2)

	return finishCandidate(name, vectorizer, lr, valTexts, valGroundTruth, devTexts, devGroundTruth, modelPath)
}

// runHumanOnlyCandidate trains using exclusively human-reviewed labels.
func runHumanOnlyCandidate(textGroups []frozen.TextGroup, trainIDs map[string]bool, reviewed map[string]int,
	idText map[string]string, valTexts []string, valGroundTruth []float64, devTexts []string, devGroundTruth []float64, modelPath string) candidateResult {

	var trainTexts []string
	var trainLabels []float64
	for _, g := range textGroups {
		for _, id := range g.IDs {
			if !trainIDs[id] {
				continue
			}
			human, ok := reviewed[id]
			if !ok {
				continue
			}
			trainTexts = append(trainTexts, idText[id])
			trainLabels = append(trainLabels, float64(human))
		}
	}
	log.Printf("candidate %q: training on %d examples (human-reviewed only)", "human", len(trainTexts))

	vectorizer := tfidf.Fit(trainTexts, trainLabels)
	X := make([][]float64, len(trainTexts))
	for i, t := range trainTexts {
		X[i] = vectorizer.Transform(t)
	}
	weights := model.BalancedWeights(trainLabels)
	lr := model.TrainWeighted(X, trainLabels, weights, epochs, learningRate, l2)

	return finishCandidate("human", vectorizer, lr, valTexts, valGroundTruth, devTexts, devGroundTruth, modelPath)
}

// runHumanOnlyMarginLoss trains using exclusively human-reviewed labels,
// like runHumanOnlyCandidate, but minimizes a margin loss
// (model.TrainMarginLoss: squared-hinge, or capped-hinge if capped) instead
// of cross-entropy. Scores are a sigmoid of the learned margin, not a
// calibrated probability, before Platt calibration. wordReserve is forwarded
// to tfidf.FitWithWordReserve (0 reproduces the default tfidf.Fit vocabulary);
// marginEpochs lets squared-hinge and capped-hinge use independently tuned
// epoch counts instead of sharing one constant.
func runHumanOnlyMarginLoss(name string, capped bool, wordReserve, marginEpochs int, textGroups []frozen.TextGroup, trainIDs map[string]bool, reviewed map[string]int,
	idText map[string]string, valTexts []string, valGroundTruth []float64, devTexts []string, devGroundTruth []float64, modelPath string) (candidateResult, error) {

	var trainTexts []string
	var trainLabels []float64
	for _, g := range textGroups {
		for _, id := range g.IDs {
			if !trainIDs[id] {
				continue
			}
			human, ok := reviewed[id]
			if !ok {
				continue
			}
			trainTexts = append(trainTexts, idText[id])
			trainLabels = append(trainLabels, float64(human))
		}
	}
	log.Printf("candidate %q: training on %d examples (human-reviewed only)", name, len(trainTexts))

	vectorizer, err := tfidf.FitWithWordReserve(trainTexts, trainLabels, 1, wordReserve)
	if err != nil {
		return candidateResult{}, err
	}
	X := make([][]float64, len(trainTexts))
	for i, t := range trainTexts {
		X[i] = vectorizer.Transform(t)
	}
	weights := model.BalancedWeights(trainLabels)
	lr, err := model.TrainMarginLoss(X, trainLabels, weights, marginEpochs, squaredHingeL2, capped)
	if err != nil {
		return candidateResult{}, err
	}

	return finishCandidate(name, vectorizer, lr, valTexts, valGroundTruth, devTexts, devGroundTruth, modelPath), nil
}

func finishCandidate(name string, vectorizer *tfidf.Vectorizer, lr *model.LogisticRegression,
	valTexts []string, valGroundTruth []float64, devTexts []string, devGroundTruth []float64, modelPath string) candidateResult {

	valScores := make([]float64, len(valTexts))
	for i, t := range valTexts {
		valScores[i] = lr.Predict(vectorizer.Transform(t))
	}
	threshold, reason := evalmetrics.PickThreshold(valScores, valGroundTruth, targetPrecision, minRecall)
	log.Printf("candidate %q: chosen threshold %.2f on validation — %s", name, threshold, reason)

	vAcc, vPrec, vRec, vF1 := evalmetrics.ConfusionAt(valScores, valGroundTruth, threshold)
	vtp, vfp, vtn, vfn := evalmetrics.ConfusionCounts(valScores, valGroundTruth, threshold)

	calibration := model.FitPlattScaling(valScores, valGroundTruth, calibrationEpochs, calibrationLearningRate)

	devScores := make([]float64, len(devTexts))
	for i, t := range devTexts {
		devScores[i] = lr.Predict(vectorizer.Transform(t))
	}
	dAcc, dPrec, dRec, dF1 := evalmetrics.ConfusionAt(devScores, devGroundTruth, threshold)
	dtp, dfp, dtn, dfn := evalmetrics.ConfusionCounts(devScores, devGroundTruth, threshold)

	m := &model.Model{
		Vocabulary:  vectorizer.Vocabulary,
		IDF:         vectorizer.IDF,
		Weights:     lr.Weights,
		Bias:        lr.Bias,
		Threshold:   threshold,
		Calibration: calibration,
		Version:     "provisional-" + name + "-" + time.Now().Format("2006-01-02T15-04-05"),
	}
	if err := model.Save(modelPath, m); err != nil {
		log.Fatalf("candidate %q: failed to save model: %v", name, err)
	}
	log.Printf("candidate %q: saved provisional model to %s (version %s)", name, modelPath, m.Version)

	return candidateResult{
		name:      name,
		threshold: threshold,
		val:       metricSet{n: len(valTexts), accuracy: vAcc, precision: vPrec, recall: vRec, f1: vF1, tp: vtp, fp: vfp, tn: vtn, fn: vfn},
		dev:       metricSet{n: len(devTexts), accuracy: dAcc, precision: dPrec, recall: dRec, f1: dF1, tp: dtp, fp: dfp, tn: dtn, fn: dfn},
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultValue
}
