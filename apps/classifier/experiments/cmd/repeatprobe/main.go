package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/classify"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/model"
	"math"
	"os"
)

type fixture struct {
	ID            string   `json:"id"`
	Label         int      `json:"label"`
	Text          string   `json:"text"`
	ExpectedScore *float64 `json:"expected_score"`
}
type result struct {
	ID              string   `json:"id"`
	Label           int      `json:"label"`
	Relevant        bool     `json:"relevant"`
	MinScore        float64  `json:"min_score"`
	MaxScore        float64  `json:"max_score"`
	UniqueVerdicts  int      `json:"unique_verdicts"`
	Stable          bool     `json:"stable"`
	ExpectedScore   *float64 `json:"expected_score,omitempty"`
	ScoreDifference *float64 `json:"score_difference,omitempty"`
}

func main() {
	mp := flag.String("model", "/data/model.json", "model")
	fp := flag.String("fixtures", "/tmp/bwt-nlp-fixtures.json", "fixtures")
	repeat := flag.Int("repeat", 30, "repetitions")
	flag.Parse()
	if *repeat < 1 {
		fmt.Fprintln(os.Stderr, "repeat must be positive")
		os.Exit(2)
	}
	b, e := os.ReadFile(*fp)
	if e != nil {
		panic(e)
	}
	var fs []fixture
	if e = json.Unmarshal(b, &fs); e != nil {
		panic(e)
	}
	mb, e := os.ReadFile(*mp)
	if e != nil {
		panic(e)
	}
	results := make([]result, len(fs))
	seen := make([]map[bool]bool, len(fs))
	for i, f := range fs {
		results[i] = result{ID: f.ID, Label: f.Label, MinScore: math.Inf(1), MaxScore: math.Inf(-1)}
		results[i].ExpectedScore = f.ExpectedScore
		seen[i] = map[bool]bool{}
	}
	var version string
	var threshold float64
	for round := 0; round < *repeat; round++ {
		m, e := model.Load(*mp)
		if e != nil {
			panic(e)
		}
		version = m.Version
		threshold = m.Threshold
		c := classify.New(m)
		for i, f := range fs {
			v := c.Classify(f.Text)
			r := &results[i]
			if round == 0 {
				r.Relevant = v.Relevant
			}
			seen[i][v.Relevant] = true
			r.MinScore = math.Min(r.MinScore, v.Score)
			r.MaxScore = math.Max(r.MaxScore, v.Score)
		}
	}
	tp, fpn, tn, fn := 0, 0, 0, 0
	stable := true
	for i := range results {
		r := &results[i]
		if r.ExpectedScore != nil {
			d := r.MinScore - *r.ExpectedScore
			r.ScoreDifference = &d
		}
		r.UniqueVerdicts = len(seen[i])
		r.Stable = r.UniqueVerdicts == 1 && r.MaxScore-r.MinScore < 1e-12
		stable = stable && r.Stable
		if r.Label == 1 {
			if r.Relevant {
				tp++
			} else {
				fn++
			}
		} else {
			if r.Relevant {
				fpn++
			} else {
				tn++
			}
		}
	}
	div := func(a, b int) float64 {
		if b == 0 {
			return 0
		}
		return float64(a) / float64(b)
	}
	out := map[string]any{"model_version": version, "model_sha256": fmt.Sprintf("%x", sha256.Sum256(mb)), "threshold": threshold, "repetitions_per_text": *repeat, "unique_texts": len(fs), "inferences": len(fs) * (*repeat), "all_decisions_stable": stable, "true_positives": tp, "false_positives": fpn, "true_negatives": tn, "false_negatives": fn, "precision": div(tp, tp+fpn), "recall": div(tp, tp+fn), "f1": div(2*tp, 2*tp+fpn+fn), "label_provenance": "assistant-authored synthetic diagnostic; not an independent model evaluation", "results": results}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if e := enc.Encode(out); e != nil {
		panic(e)
	}
}
