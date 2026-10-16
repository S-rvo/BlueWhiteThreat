// apps/classifier/cmd/train/main.go
package main

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/model"
	textutil "github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/text"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/tfidf"
	"github.com/joho/godotenv"
)

const (
	trainSplitRatio = 0.8
	epochs          = 1500
	learningRate    = 2.0
	l2              = 0.0001
	splitSeed       = 42
	// targetPrecision is the precision floor we try to hit when picking a
	// decision threshold: among thresholds that reach it on the eval set,
	// we keep the one with the best recall. If no threshold reaches it, we
	// fall back to the threshold that maximizes F1.
	targetPrecision = 0.95
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env found or unreadable, continuing with existing environment.")
	}

	inputPath := getEnvOrDefault("TRAINING_DATA", "training_data.csv")
	outputPath := getEnvOrDefault("MODEL_PATH", "model.json")

	texts, labels, err := readLabeledCSV(inputPath)
	if err != nil {
		log.Fatalf("failed to read training data: %v", err)
	}
	if len(texts) < 10 {
		log.Fatalf("need at least 10 labeled examples, got %d — label more data first", len(texts))
	}

	// The final test set never participates in feature fitting or selection.
	developmentTexts, developmentLabels, testTexts, testLabels := split(texts, labels, trainSplitRatio)
	trainTexts, trainLabels, evalTexts, evalLabels := split(developmentTexts, developmentLabels, 0.75)
	for name, ys := range map[string][]float64{"train": trainLabels, "validation": evalLabels, "test": testLabels} {
		var positives int
		for _, y := range ys {
			if y == 1 {
				positives++
			}
		}
		if positives < 2 || len(ys)-positives < 2 {
			log.Fatalf("%s needs at least 2 unique examples of each class", name)
		}
	}
	log.Printf("Deduplicated split: train=%d validation=%d test=%d (seed=%d)", len(trainTexts), len(evalTexts), len(testTexts), splitSeed)
	var vectorizer *tfidf.Vectorizer
	var bestVectorizer *tfidf.Vectorizer
	balancedWeights := model.BalancedWeights(trainLabels)
	var lr *model.LogisticRegression
	var chosenThreshold float64
	bestF1 := -1.0
	bestMeetsTarget := false
	bestSublinear := false
	for _, characters := range []bool{false, true} {
		vectorizer = tfidf.FitWithCharacters(trainTexts, trainLabels, characters)
		for _, sublinear := range []bool{false, true} {
			vectorizer.SublinearTF = sublinear
			X := make([][]float64, len(trainTexts))
			for i, t := range trainTexts {
				X[i] = vectorizer.Transform(t)
			}
			for _, weights := range [][]float64{balancedWeights, nil} {
				for _, regularization := range []float64{0.00001, l2, 0.001} {
					candidate := model.TrainWeighted(X, trainLabels, weights, epochs, learningRate, regularization)
					scores := predictAll(candidate, vectorizer, evalTexts)
					threshold := pickThreshold(scores, evalLabels)
					_, precision, _, f1 := confusionAt(scores, evalLabels, threshold)
					meets := precision >= targetPrecision
					log.Printf("Candidate characters=%t sublinear=%t balanced=%t l2=%g validation F1=%.4f precision=%.4f", characters, sublinear, weights != nil, regularization, f1, precision)
					if (meets && !bestMeetsTarget) || (meets == bestMeetsTarget && f1 > bestF1) {
						lr, chosenThreshold, bestF1 = candidate, threshold, f1
						bestSublinear, bestMeetsTarget = sublinear, meets
						bestVectorizer = vectorizer
					}
				}
			}
		}
	}
	vectorizer = bestVectorizer
	vectorizer.SublinearTF = bestSublinear
	testScores := predictAll(lr, vectorizer, testTexts)

	m := &model.Model{
		CharNgrams:  vectorizer.CharNgrams,
		SublinearTF: bestSublinear,
		Vocabulary:  vectorizer.Vocabulary,
		IDF:         vectorizer.IDF,
		Weights:     lr.Weights,
		Bias:        lr.Bias,
		Threshold:   chosenThreshold,
		Version:     time.Now().Format("2006-01-02T15-04-05"),
	}

	log.Println("Independent held-out test (not used for model/threshold selection):")
	evaluateAt(testScores, testLabels, chosenThreshold)
	_, p, r, f := confusionAt(testScores, testLabels, chosenThreshold)
	log.Printf("95%% precision/recall/F1 target reached: %t", p >= .95 && r >= .95 && f >= .95)

	if err := model.Save(outputPath, m); err != nil {
		log.Fatalf("failed to save model: %v", err)
	}
	log.Printf("Saved model to %s (version %s)", outputPath, m.Version)
	if err := writeReport(outputPath+".evaluation.json", m, testTexts, testLabels, testScores); err != nil {
		log.Fatalf("failed to write evaluation report: %v", err)
	}
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

	skipped := 0
	seen := make(map[string]float64)
	conflicts := make(map[string]bool)
	for _, row := range rows[1:] { // skip header
		// Supports both the legacy 2-column format (text,label) and the
		// current 3-column format (id,text,label) — text and label are
		// always the last two fields.
		if len(row) != 2 && len(row) != 3 {
			skipped++
			continue
		}
		text := row[len(row)-2]
		label, err := strconv.ParseFloat(row[len(row)-1], 64)
		if err != nil || (label != 0 && label != 1) {
			return nil, nil, fmt.Errorf("invalid binary label in data row %d", len(texts)+1)
		}
		key := strings.Join(textutil.Tokenize(text), " ")
		if key == "" {
			skipped++
			continue
		}
		if previous, ok := seen[key]; ok {
			if previous != label {
				conflicts[key] = true
			}
			skipped++
			continue
		}
		seen[key] = label
		texts = append(texts, text)
		labels = append(labels, label)
	}
	if len(conflicts) > 0 {
		cleanTexts, cleanLabels := []string{}, []float64{}
		for i, doc := range texts {
			if !conflicts[strings.Join(textutil.Tokenize(doc), " ")] {
				cleanTexts = append(cleanTexts, doc)
				cleanLabels = append(cleanLabels, labels[i])
			}
		}
		texts, labels = cleanTexts, cleanLabels
		log.Printf("Excluded %d conflicting duplicate groups; resolve their labels before reuse", len(conflicts))
	}
	if skipped > 0 {
		log.Printf("Removed %d duplicate, empty or malformed rows out of %d", skipped, len(rows)-1)
	}
	return texts, labels, nil
}

// split performs a stratified train/eval split so both classes are
// represented proportionally in the (small) eval set — a plain random split
// on a few dozen examples can leave the eval set with almost no positives,
// making precision/recall/f1 swing wildly between runs.
func split(texts []string, labels []float64, trainRatio float64) (trainTexts []string, trainLabels []float64, evalTexts []string, evalLabels []float64) {
	rng := rand.New(rand.NewSource(splitSeed))

	var classes = map[float64][]int{}
	for i, l := range labels {
		classes[l] = append(classes[l], i)
	}

	// Go map iteration order is randomized per run, which would otherwise make
	// the RNG stream order (and thus the resulting split) different on every
	// run despite the fixed splitSeed. Sort class keys for a stable order.
	keys := make([]float64, 0, len(classes))
	for k := range classes {
		keys = append(keys, k)
	}
	sort.Float64s(keys)

	for _, k := range keys {
		idxs := classes[k]
		perm := rng.Perm(len(idxs))
		cut := int(float64(len(idxs)) * trainRatio)
		for i, p := range perm {
			idx := idxs[p]
			if i < cut {
				trainTexts = append(trainTexts, texts[idx])
				trainLabels = append(trainLabels, labels[idx])
			} else {
				evalTexts = append(evalTexts, texts[idx])
				evalLabels = append(evalLabels, labels[idx])
			}
		}
	}
	return
}

// confusionAt computes accuracy/precision/recall/f1 for scores classified
// as positive when score >= threshold, against the actual labels.
func confusionAt(scores []float64, labels []float64, threshold float64) (accuracy, precision, recall, f1 float64) {
	var truePositive, falsePositive, trueNegative, falseNegative int
	for i, score := range scores {
		predicted := 0
		if score >= threshold {
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

	total := len(scores)
	accuracy = safeDiv(float64(truePositive+trueNegative), float64(total))
	precision = safeDiv(float64(truePositive), float64(truePositive+falsePositive))
	recall = safeDiv(float64(truePositive), float64(truePositive+falseNegative))
	f1 = safeDiv(2*precision*recall, precision+recall)
	return
}

// pickThreshold sweeps candidate thresholds against the eval set and logs
// a precision/recall table, so the chosen cutoff is visible and auditable
// instead of being a blind guess. Among thresholds that reach
// targetPrecision, it keeps the one with the highest recall (the least
// aggressive way to hit the precision floor); if none reach it, it falls
// back to the threshold that maximizes F1.
func pickThreshold(scores []float64, labels []float64) float64 {
	type candidate struct {
		threshold                       float64
		accuracy, precision, recall, f1 float64
	}

	var candidates []candidate
	thresholds := append([]float64{0}, scores...)
	sort.Float64s(thresholds)
	if len(scores) > 0 {
		thresholds = append(thresholds, math.Nextafter(thresholds[len(thresholds)-1], math.Inf(1)))
	}
	for _, t := range thresholds {
		accuracy, precision, recall, f1 := confusionAt(scores, labels, t)
		candidates = append(candidates, candidate{t, accuracy, precision, recall, f1})
	}

	log.Println("Threshold sweep (eval set):")
	for _, t := range []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9} {
		_, p, r, f := confusionAt(scores, labels, t)
		log.Printf("  threshold %.2f — precision: %.3f, recall: %.3f, f1: %.3f", t, p, r, f)
	}

	bestByPrecisionFloor := -1
	bestByF1 := 0
	for i, c := range candidates {
		if c.precision >= targetPrecision {
			if bestByPrecisionFloor == -1 || c.recall > candidates[bestByPrecisionFloor].recall {
				bestByPrecisionFloor = i
			}
		}
		if c.f1 > candidates[bestByF1].f1 {
			bestByF1 = i
		}
	}

	if bestByPrecisionFloor != -1 {
		c := candidates[bestByPrecisionFloor]
		log.Printf("Chosen threshold %.2f reaches target precision %.2f (precision: %.3f, recall: %.3f, f1: %.3f)",
			c.threshold, targetPrecision, c.precision, c.recall, c.f1)
		return c.threshold
	}

	c := candidates[bestByF1]
	log.Printf("No threshold on the eval set reaches target precision %.2f; falling back to the F1-maximizing threshold %.2f (precision: %.3f, recall: %.3f, f1: %.3f)",
		targetPrecision, c.threshold, c.precision, c.recall, c.f1)
	return c.threshold
}

func evaluateAt(scores []float64, labels []float64, threshold float64) {
	accuracy, precision, recall, f1 := confusionAt(scores, labels, threshold)
	log.Printf("Eval metrics @ threshold %.2f — accuracy: %.3f, precision: %.3f, recall: %.3f, f1: %.3f (n=%d)",
		threshold, accuracy, precision, recall, f1, len(scores))
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

func predictAll(lr *model.LogisticRegression, v *tfidf.Vectorizer, texts []string) []float64 {
	scores := make([]float64, len(texts))
	for i, text := range texts {
		scores[i] = lr.Predict(v.Transform(text))
	}
	return scores
}

// Reports contain hashes rather than scraped content; errors can be matched
// locally to source pages without copying their contents into artifacts.
func writeReport(path string, m *model.Model, texts []string, labels, scores []float64) error {
	type mistake struct {
		SHA256 string  `json:"text_sha256"`
		Label  float64 `json:"label"`
		Score  float64 `json:"score"`
	}
	a, p, r, f := confusionAt(scores, labels, m.Threshold)
	errors := []mistake{}
	positives := 0
	for i, score := range scores {
		if labels[i] == 1 {
			positives++
		}
		if (score >= m.Threshold) != (labels[i] == 1) {
			errors = append(errors, mistake{fmt.Sprintf("%x", sha256.Sum256([]byte(texts[i]))), labels[i], score})
		}
	}
	report := struct {
		Version       string    `json:"model_version"`
		Seed          int       `json:"split_seed"`
		Samples       int       `json:"test_samples"`
		Positives     int       `json:"test_positives"`
		Accuracy      float64   `json:"accuracy"`
		Precision     float64   `json:"precision"`
		Recall        float64   `json:"recall"`
		F1            float64   `json:"f1"`
		TargetReached bool      `json:"target_95_reached"`
		Threshold     float64   `json:"threshold"`
		Mistakes      []mistake `json:"mistakes"`
	}{m.Version, splitSeed, len(texts), positives, a, p, r, f, p >= .95 && r >= .95 && f >= .95, m.Threshold, errors}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}
