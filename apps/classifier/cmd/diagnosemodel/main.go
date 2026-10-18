// diagnosemodel isolates threshold resolution and L2 strength on the existing
// human validation split. It never selects using the public diagnostic labels.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/confirmation"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/evalmetrics"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/frozen"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/model"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/review"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/text"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/tfidf"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/threshold"
)

type metrics struct {
	N, TP, FP, TN, FN                                   int
	Precision, Recall, F1, Accuracy, MinScore, MaxScore float64
}

func measure(scores, labels []float64, t float64) metrics {
	tp, fp, tn, fn := evalmetrics.ConfusionCounts(scores, labels, t)
	a, p, r, f := evalmetrics.ConfusionAt(scores, labels, t)
	lo, hi := 1.0, 0.0
	for _, s := range scores {
		lo = math.Min(lo, s)
		hi = math.Max(hi, s)
	}
	return metrics{len(scores), tp, fp, tn, fn, p, r, f, a, lo, hi}
}

type result struct {
	Name                                    string
	L2, Threshold, WeightNorm, GradientNorm float64
	Train, Validation                       metrics
}

func predict(lr *model.LogisticRegression, X [][]float64) []float64 {
	s := make([]float64, len(X))
	for i, x := range X {
		s[i] = lr.Predict(x)
	}
	return s
}
func gradient(lr *model.LogisticRegression, X [][]float64, y, w []float64, l2 float64) float64 {
	g := make([]float64, len(lr.Weights))
	b, total := 0.0, 0.0
	for i, x := range X {
		e := (lr.Predict(x) - y[i]) * w[i]
		b += e
		total += w[i]
		for j, v := range x {
			g[j] += e * v
		}
	}
	sum := math.Pow(b/total, 2)
	for j, v := range g {
		sum += math.Pow(v/total+l2*lr.Weights[j], 2)
	}
	return math.Sqrt(sum)
}
func norm(w []float64) float64 {
	s := 0.0
	for _, v := range w {
		s += v * v
	}
	return math.Sqrt(s)
}
func write(path string, v interface{}) error {
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
func run() error {
	out := flag.String("out", "data/score-investigation-20261013", "new output directory")
	retrain := flag.Bool("retrain", false, "compare predeclared L2 values 0.05 and 0.005, 4000 epochs")
	flag.Parse()
	c, err := confirmation.Load("data/confirmation-campaign", ".")
	if err != nil {
		return err
	}
	reviewedLabels, err := review.LoadReviewed("data/reviewed_labels.csv")
	if err != nil {
		return err
	}
	reviewed := map[string]int{}
	for id, l := range reviewedLabels {
		reviewed[id] = l.Label
	}
	ids, texts, labels := []string{}, []string{}, map[string]float64{}
	byID := map[string]string{}
	for _, d := range c.Corpus {
		ids = append(ids, d.ID)
		texts = append(texts, d.Text)
		labels[d.ID] = d.Label
		byID[d.ID] = d.Text
	}
	groups := frozen.GroupByTextOnly(ids, texts)
	trainIDs, valIDs, testIDs := frozen.Partition(groups, labels, 42, .7, .15)
	// Preserve the original training order to reproduce the frozen candidate.
	var trainTexts, valTexts []string
	var trainY, valY []float64
	var trainUsed, valUsed []string
	for _, g := range groups {
		for _, id := range g.IDs {
			if label, ok := reviewed[id]; ok && trainIDs[id] {
				trainTexts = append(trainTexts, byID[id])
				trainY = append(trainY, float64(label))
				trainUsed = append(trainUsed, id)
			}
		}
	}
	sortedIDs := append([]string(nil), ids...)
	sort.Strings(sortedIDs)
	for _, id := range sortedIDs {
		if label, ok := reviewed[id]; ok && valIDs[id] {
			valTexts = append(valTexts, byID[id])
			valY = append(valY, float64(label))
			valUsed = append(valUsed, id)
		}
	}
	if len(trainY) != 573 || len(valY) != 116 || len(testIDs) != 347 {
		return fmt.Errorf("partition changed: human train=%d val=%d dev=%d", len(trainY), len(valY), len(testIDs))
	}
	fitted := tfidf.Fit(trainTexts, trainY)
	for tok, i := range c.Model.Vocabulary {
		if j, ok := fitted.Vocabulary[tok]; !ok || i != j || fitted.IDF[j] != c.Model.IDF[i] {
			return fmt.Errorf("training vocabulary changed")
		}
	}
	if len(fitted.Vocabulary) != len(c.Model.Vocabulary) {
		return fmt.Errorf("vocabulary length changed")
	}
	v := &tfidf.Vectorizer{Vocabulary: c.Model.Vocabulary, IDF: c.Model.IDF}
	transform := func(ts []string) [][]float64 {
		X := make([][]float64, len(ts))
		for i, t := range ts {
			X[i] = v.Transform(t)
		}
		return X
	}
	X, V := transform(trainTexts), transform(valTexts)
	weights := model.BalancedWeights(trainY)
	if err = os.Mkdir(*out, 0700); err != nil {
		return err
	}
	reviewedBytes, err := os.ReadFile("data/reviewed_labels.csv")
	if err != nil {
		return err
	}
	if err = write(filepath.Join(*out, "protocol.json"), map[string]interface{}{"selection": "existing human validation only; no public labels and no dev/test scores", "train_ids": trainUsed, "val_ids": valUsed, "model_sha256": c.Manifest.ModelSHA256, "reviewed_labels_sha256": fmt.Sprintf("%x", sha256.Sum256(reviewedBytes)), "l2_grid": []float64{.5, .05, .005}, "epochs": 4000, "learning_rate": 2, "target_precision": .99, "min_recall": .3}); err != nil {
		return err
	}
	var results []result
	evaluate := func(name string, lr *model.LogisticRegression, l2, t float64) result {
		r := result{name, l2, t, norm(lr.Weights), gradient(lr, X, trainY, weights, l2), measure(predict(lr, X), trainY, t), measure(predict(lr, V), valY, t)}
		log.Printf("%s threshold=%.9f norm=%.5f gradient=%.3g val P=%.3f R=%.3f F1=%.3f", name, t, r.WeightNorm, r.GradientNorm, r.Validation.Precision, r.Validation.Recall, r.Validation.F1)
		results = append(results, r)
		return r
	}
	lr := &model.LogisticRegression{Weights: c.Model.Weights, Bias: c.Model.Bias}
	evaluate("frozen-grid", lr, .5, c.Model.Threshold)
	t, err := threshold.Pick(predict(lr, V), valY, .99, .3)
	if err != nil {
		return err
	}
	evaluate("frozen-exact", lr, .5, t)
	// Diagnose feature support/contributions without changing labels or thresholds.
	b, err := os.ReadFile("data/assistant-diagnostic-20261013/collection.json")
	if err != nil {
		return err
	}
	var public []struct{ ID, URL, Text string }
	if err = json.Unmarshal(b, &public); err != nil {
		return err
	}
	reverse := make([]string, len(v.Vocabulary))
	for tok, i := range v.Vocabulary {
		reverse[i] = tok
	}
	var details []interface{}
	for _, d := range public {
		if d.Text == "" {
			continue
		}
		x := v.Transform(d.Text)
		matched, total := 0, 0
		for _, tok := range text.Tokenize(d.Text) {
			total++
			if _, ok := v.Vocabulary[tok]; ok {
				matched++
			}
		}
		type contribution struct {
			Term  string
			Value float64
		}
		var terms []contribution
		for j, val := range x {
			if val != 0 {
				terms = append(terms, contribution{reverse[j], val * lr.Weights[j]})
			}
		}
		sort.Slice(terms, func(i, j int) bool { return terms[i].Value < terms[j].Value })
		n := len(terms)
		k := 5
		if n < k {
			k = n
		}
		details = append(details, map[string]interface{}{"id": d.ID, "url": d.URL, "word_occurrences": total, "known_word_occurrences": matched, "active_features": n, "score": lr.Predict(x), "bias": lr.Bias, "negative_contributions": terms[:k], "positive_contributions": terms[n-k:]})
	}
	if err = write(filepath.Join(*out, "feature-diagnostics.json"), details); err != nil {
		return err
	}
	if *retrain {
		for _, l2 := range []float64{.05, .005} {
			log.Printf("Training L2=%g on %d human documents", l2, len(trainY))
			candidate := model.TrainWeighted(X, trainY, weights, 4000, 2, l2)
			scores := predict(candidate, V)
			grid, _ := evalmetrics.PickThreshold(scores, valY, .99, .3)
			name := fmt.Sprintf("l2-%g", l2)
			evaluate(name+"-grid", candidate, l2, grid)
			exact, e := threshold.Pick(scores, valY, .99, .3)
			if e != nil {
				return e
			}
			evaluate(name+"-exact", candidate, l2, exact)
			m := *c.Model
			m.Weights = candidate.Weights
			m.Bias = candidate.Bias
			m.Threshold = exact
			m.Version = "provisional-diagnostic-" + name
			m.Calibration = model.FitPlattScaling(scores, valY, 2000, .1)
			if err = model.Save(filepath.Join(*out, name+"-model.json"), &m); err != nil {
				return err
			}
		}
	}
	return write(filepath.Join(*out, "results.json"), results)
}
func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
