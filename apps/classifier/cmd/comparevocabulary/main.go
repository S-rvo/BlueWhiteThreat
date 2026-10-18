// comparevocabulary runs a predeclared vocabulary ablation on human validation.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/confirmation"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/evalmetrics"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/frozen"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/model"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/overlap"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/review"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/tfidf"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/threshold"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/vocabexperiment"
	"log"
	"os"
	"path/filepath"
)

type point struct {
	Threshold, Precision, Recall, F1 float64
	TP, FP, TN, FN                   int
}

func metric(s, y []float64, t float64) point {
	_, p, r, f := evalmetrics.ConfusionAt(s, y, t)
	tp, fp, tn, fn := evalmetrics.ConfusionCounts(s, y, t)
	return point{t, p, r, f, tp, fp, tn, fn}
}

// precisionPoint applies the same predeclared recall floor to every candidate.
func precisionPoint(s, y []float64) point {
	best := point{Precision: -1}
	for _, t := range s {
		p := metric(s, y, t)
		if p.Recall >= .6 && (p.Precision > best.Precision || (p.Precision == best.Precision && p.Recall > best.Recall)) {
			best = p
		}
	}
	return best
}
func write(path string, v interface{}) error {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

type result struct {
	Name                            string
	Vocabulary                      int
	L2                              float64
	F1Fallback, PrecisionAtRecall60 point
	CTITerms                        map[string]bool
}

func run() error {
	out := flag.String("out", "data/vocabulary-experiment-20261013", "new experiment directory")
	purge := flag.Bool("purge-overlap", false, "remove human train texts overlapping any frozen validation/dev text before fitting")
	flag.Parse()
	campaign, e := confirmation.Load("data/confirmation-campaign", ".")
	if e != nil {
		return e
	}
	b, e := os.ReadFile("data/score-investigation-20261013/protocol.json")
	if e != nil {
		return e
	}
	var protocol struct {
		TrainIDs     []string `json:"train_ids"`
		ValIDs       []string `json:"val_ids"`
		ReviewedHash string   `json:"reviewed_labels_sha256"`
	}
	if e = json.Unmarshal(b, &protocol); e != nil {
		return e
	}
	b, e = os.ReadFile("data/reviewed_labels.csv")
	if e != nil {
		return e
	}
	if fmt.Sprintf("%x", sha256.Sum256(b)) != protocol.ReviewedHash {
		return fmt.Errorf("reviewed labels changed")
	}
	reviewedLabels, e := review.LoadReviewed("data/reviewed_labels.csv")
	if e != nil {
		return e
	}
	reviewed := map[string]int{}
	for id, l := range reviewedLabels {
		reviewed[id] = l.Label
	}
	byID := map[string]string{}
	for _, d := range campaign.Corpus {
		byID[d.ID] = d.Text
	}
	var train, val []string
	var y, vy []float64
	seen := map[string]bool{}
	for _, id := range protocol.TrainIDs {
		t, ok := byID[id]
		l, has := reviewed[id]
		if !ok || !has || seen[id] {
			return fmt.Errorf("invalid train id")
		}
		seen[id] = true
		train = append(train, t)
		y = append(y, float64(l))
	}
	for _, id := range protocol.ValIDs {
		t, ok := byID[id]
		l, has := reviewed[id]
		if !ok || !has || seen[id] {
			return fmt.Errorf("invalid validation id")
		}
		seen[id] = true
		val = append(val, t)
		vy = append(vy, float64(l))
	}
	if len(y) != 573 || len(vy) != 116 {
		return fmt.Errorf("unexpected split")
	}
	var removed []map[string]string
	if *purge {
		var ids, texts []string
		labels := map[string]float64{}
		for _, d := range campaign.Corpus {
			ids = append(ids, d.ID)
			texts = append(texts, d.Text)
			labels[d.ID] = d.Label
		}
		_, valIDs, devIDs := frozen.Partition(frozen.GroupByTextOnly(ids, texts), labels, 42, .7, .15)
		var refs []overlap.Print
		var refIDs []string
		for _, d := range campaign.Corpus {
			if valIDs[d.ID] || devIDs[d.ID] {
				refs = append(refs, overlap.Fingerprint(d.Text))
				refIDs = append(refIDs, d.ID)
			}
		}
		var keptIDs, keptTexts []string
		var keptY []float64
		for i, id := range protocol.TrainIDs {
			p := overlap.Fingerprint(train[i])
			match := ""
			for j, ref := range refs {
				if overlap.Similar(p, ref) {
					match = refIDs[j]
					break
				}
			}
			if match != "" {
				removed = append(removed, map[string]string{"train_id": id, "heldout_id": match})
				continue
			}
			keptIDs = append(keptIDs, id)
			keptTexts = append(keptTexts, train[i])
			keptY = append(keptY, y[i])
		}
		log.Printf("Overlap purge: removed %d/%d train docs against %d held-out texts; validation remains %d", len(removed), len(train), len(refs), len(val))
		protocol.TrainIDs = keptIDs
		train = keptTexts
		y = keptY
		if len(y) == 0 {
			return fmt.Errorf("purge removed all training documents")
		}
	}
	if e = os.Mkdir(*out, 0700); e != nil {
		return e
	}
	vocabularies := []string{"frozen-chi2", "word-df", "word-bigram-df"}
	if *purge {
		vocabularies = []string{"purged-chi2", "purged-word"}
	}
	if e = write(filepath.Join(*out, "protocol.json"), map[string]interface{}{"partition": protocol, "vocabularies": vocabularies, "l2": []float64{.5, .005}, "epochs": 4000, "learning_rate": 2, "primary_objective": "maximum validation precision with recall >= 0.60; ties recall then F1", "secondary": "existing precision .99 / recall .3 rule, fallback F1", "no_public_or_dev_scoring": true, "purge_overlap": *purge, "removed_train_matches": removed, "purge_rule": "normalized exact or five-token Jaccard >=0.8 with >=20 tokens; compare to all frozen val/dev texts", "purged_candidates": "chi2 L2=.5 and word DF L2=.005 only"}); e != nil {
		return e
	}
	var results []result
	evaluate := func(name string, v *tfidf.Vectorizer, lr *model.LogisticRegression, l2 float64) error {
		s := make([]float64, len(val))
		for i, t := range val {
			s[i] = lr.Predict(v.Transform(t))
		}
		cut, err := threshold.Pick(s, vy, .99, .3)
		if err != nil {
			return err
		}
		r := result{name, len(v.Vocabulary), l2, metric(s, vy, cut), precisionPoint(s, vy), map[string]bool{}}
		for _, word := range []string{"malware", "ransomware", "vulnerability", "vulnerabilities", "cve", "exploit", "breach"} {
			_, r.CTITerms[word] = v.Vocabulary[word]
		}
		results = append(results, r)
		p := r.PrecisionAtRecall60
		log.Printf("%s V=%d precision objective P=%.3f R=%.3f F1=%.3f (TP=%d FP=%d); F1 fallback %.3f", name, r.Vocabulary, p.Precision, p.Recall, p.F1, p.TP, p.FP, r.F1Fallback.F1)
		m := &model.Model{Vocabulary: v.Vocabulary, IDF: v.IDF, Weights: lr.Weights, Bias: lr.Bias, Threshold: p.Threshold, Calibration: model.FitPlattScaling(s, vy, 2000, .1), Version: "provisional-vocabulary-" + name}
		if err := model.Save(filepath.Join(*out, name+"-model.json"), m); err != nil {
			return err
		}
		m.Threshold = r.F1Fallback.Threshold
		m.Version += "-f1"
		return model.Save(filepath.Join(*out, name+"-f1-model.json"), m)
	}
	if *purge {
		for _, kind := range []string{"chi2", "word"} {
			v := tfidf.Fit(train, y)
			l2 := .5
			if kind == "word" {
				v = vocabexperiment.Fit(train, false)
				l2 = .005
			}
			X := make([][]float64, len(train))
			for i, t := range train {
				X[i] = v.Transform(t)
			}
			log.Printf("Training purged %s L2=%g n=%d", kind, l2, len(y))
			lr := model.TrainWeighted(X, y, model.BalancedWeights(y), 4000, 2, l2)
			if e = evaluate("purged-"+kind, v, lr, l2); e != nil {
				return e
			}
		}
	} else {
		base := campaign.Model
		if e = evaluate("frozen-l2-0.5", &tfidf.Vectorizer{Vocabulary: base.Vocabulary, IDF: base.IDF}, &model.LogisticRegression{Weights: base.Weights, Bias: base.Bias}, .5); e != nil {
			return e
		}
		previous, e := model.Load("data/score-investigation-20261013/l2-0.005-model.json")
		if e != nil {
			return e
		}
		if e = evaluate("frozen-l2-0.005", &tfidf.Vectorizer{Vocabulary: previous.Vocabulary, IDF: previous.IDF}, &model.LogisticRegression{Weights: previous.Weights, Bias: previous.Bias}, .005); e != nil {
			return e
		}
		for _, bigrams := range []bool{false, true} {
			name := "word"
			if bigrams {
				name = "word-bigram"
			}
			v := vocabexperiment.Fit(train, bigrams)
			X := make([][]float64, len(train))
			for i, t := range train {
				X[i] = v.Transform(t)
			}
			for _, l2 := range []float64{.5, .005} {
				log.Printf("Training %s L2=%g", name, l2)
				lr := model.TrainWeighted(X, y, model.BalancedWeights(y), 4000, 2, l2)
				if e = evaluate(fmt.Sprintf("%s-l2-%g", name, l2), v, lr, l2); e != nil {
					return e
				}
			}
		}

	}
	if e = write(filepath.Join(*out, "results.json"), results); e != nil {
		return e
	}
	best := 0
	for i, r := range results {
		p, q := r.PrecisionAtRecall60, results[best].PrecisionAtRecall60
		if p.Precision > q.Precision || (p.Precision == q.Precision && p.Recall > q.Recall) || (p.Precision == q.Precision && p.Recall == q.Recall && p.F1 > q.F1) {
			best = i
		}
	}
	return write(filepath.Join(*out, "selection.json"), map[string]interface{}{
		"status":    "provisional; validation selection, not independent confirmation",
		"objective": "maximum precision with recall >= 0.60; ties recall then F1",
		"selected":  results[best].Name,
		"metrics":   results[best].PrecisionAtRecall60,
		"promoted":  false,
	})
}
func main() {
	if e := run(); e != nil {
		log.Fatal(e)
	}
}
