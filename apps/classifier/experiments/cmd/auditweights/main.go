// apps/classifier/experiments/cmd/auditweights/main.go
//
// auditweights is the linear-model analogue of "open the weights, find the
// neuron that's causing trouble, fix it": unlike a deep net, this model has
// no superposition, so every feature's effect on the score is already exact
// and legible — weight[i] times that feature's TF-IDF value, full stop. This
// tool ranks every vocabulary term by a risk score (|weight| * IDF) that
// surfaces terms which are both rare (high IDF, so possibly memorized from a
// handful of documents rather than learned as a generalizable signal) and
// influential (high |weight|). Optionally, it ablates the riskiest terms
// (zeroes their weight — a surgical edit, no retraining) and reports the
// resulting precision/recall/F1 delta on a held-out labeled CSV, so a
// suspected "bad feature" can be confirmed or ruled out with a number
// instead of a guess. Not part of the production pipeline.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/evalmetrics"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/model"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/tfidf"
)

// featureRisk describes one vocabulary term's contribution and how
// suspicious it looks: a high RiskScore means an influential term backed by
// little document support (inferred from IDF, which rises monotonically as
// document frequency falls, independent of the training corpus size).
type featureRisk struct {
	Rank      int     `json:"rank"`
	Term      string  `json:"term"`
	Index     int     `json:"index"`
	Family    string  `json:"family"`
	Weight    float64 `json:"weight"`
	IDF       float64 `json:"idf"`
	RiskScore float64 `json:"risk_score"`
	// DocFrequency is only populated when -train-docs recovers it from IDF;
	// it is an estimate (IDF is rounded in storage), not an exact count.
	DocFrequency int `json:"doc_frequency,omitempty"`
}

type familyStats struct {
	Family    string  `json:"family"`
	Count     int     `json:"count"`
	ZeroCount int     `json:"zero_count"`
	MeanAbs   float64 `json:"mean_abs_weight"`
}

type report struct {
	ModelPath      string        `json:"model_path"`
	ModelVersion   string        `json:"model_version"`
	VocabularySize int           `json:"vocabulary_size"`
	WeightNorm     float64       `json:"weight_norm"`
	ZeroWeights    int           `json:"zero_weights"`
	FamilyStats    []familyStats `json:"family_stats"`
	TopRisk        []featureRisk `json:"top_risk"`
}

// family classifies a vocabulary term the same way internal/tfidf buckets
// features for its own family-budget reservation, so this audit's notion of
// "family" matches the one the vectorizer itself uses.
func family(term string) string {
	switch {
	case strings.HasPrefix(term, "s_"):
		return "structural"
	case strings.HasPrefix(term, "c_"):
		return "char_ngram"
	case strings.Contains(term, "_"):
		return "bigram"
	default:
		return "word"
	}
}

// estimateDocFrequency inverts Fit's IDF formula idf = ln((n+1)/(df+1)) + 1
// to recover the document frequency a term was fit with, given the total
// number of training documents n. This is only as precise as the stored
// IDF's float precision; it is a diagnostic estimate, not an exact replay.
func estimateDocFrequency(idf float64, n int) int {
	if n <= 0 {
		return 0
	}
	df := (float64(n)+1)/math.Exp(idf-1) - 1
	if df < 0 {
		df = 0
	}
	return int(math.Round(df))
}

func buildReport(m *model.Model, trainDocs int, topN int) report {
	reverse := make([]string, len(m.Vocabulary))
	for term, idx := range m.Vocabulary {
		reverse[idx] = term
	}

	risks := make([]featureRisk, len(m.Weights))
	famCount := map[string]int{}
	famZero := map[string]int{}
	famAbsSum := map[string]float64{}
	var weightNorm float64
	var zeroWeights int

	for i, w := range m.Weights {
		term := reverse[i]
		fam := family(term)
		idf := m.IDF[i]
		risk := featureRisk{
			Term:      term,
			Index:     i,
			Family:    fam,
			Weight:    w,
			IDF:       idf,
			RiskScore: math.Abs(w) * idf,
		}
		if trainDocs > 0 {
			risk.DocFrequency = estimateDocFrequency(idf, trainDocs)
		}
		risks[i] = risk

		famCount[fam]++
		famAbsSum[fam] += math.Abs(w)
		if w == 0 {
			famZero[fam]++
			zeroWeights++
		}
		weightNorm += w * w
	}
	weightNorm = math.Sqrt(weightNorm)

	sort.Slice(risks, func(i, j int) bool { return risks[i].RiskScore > risks[j].RiskScore })
	for i := range risks {
		risks[i].Rank = i + 1
	}
	top := risks
	if len(top) > topN {
		top = top[:topN]
	}

	var families []string
	for fam := range famCount {
		families = append(families, fam)
	}
	sort.Strings(families)
	var stats []familyStats
	for _, fam := range families {
		stats = append(stats, familyStats{
			Family:    fam,
			Count:     famCount[fam],
			ZeroCount: famZero[fam],
			MeanAbs:   famAbsSum[fam] / float64(famCount[fam]),
		})
	}

	return report{
		ModelVersion:   m.Version,
		VocabularySize: len(m.Vocabulary),
		WeightNorm:     weightNorm,
		ZeroWeights:    zeroWeights,
		FamilyStats:    stats,
		TopRisk:        top,
	}
}

// ablate returns a copy of weights with the features named in indices zeroed
// out — a surgical edit standing in for "remove this neuron" with nothing
// else (bias, threshold, other weights) touched, so any metric delta is
// attributable to exactly those features.
func ablate(weights []float64, indices []int) []float64 {
	out := append([]float64(nil), weights...)
	for _, idx := range indices {
		out[idx] = 0
	}
	return out
}

func evaluate(m *model.Model, weights []float64, texts []string, labels []float64) (accuracy, precision, recall, f1 float64) {
	vectorizer := &tfidf.Vectorizer{Vocabulary: m.Vocabulary, IDF: m.IDF, LegacyCharNgrams: m.CharNgrams, LegacySublinearTF: m.SublinearTF}
	lr := &model.LogisticRegression{Weights: weights, Bias: m.Bias}
	scores := make([]float64, len(texts))
	for i, t := range texts {
		scores[i] = lr.Predict(vectorizer.Transform(t))
	}
	return evalmetrics.ConfusionAt(scores, labels, m.Threshold)
}

func run() error {
	modelPath := flag.String("model", "model.json", "path to the model.json to audit")
	top := flag.Int("top", 40, "number of highest-risk features to print/report")
	trainDocs := flag.Int("train-docs", 0, "total training document count, to additionally estimate document frequency from IDF (0 = skip)")
	evalPath := flag.String("eval", "", "optional id,text,label CSV to measure the effect of ablating the top-risk features")
	ablateN := flag.Int("ablate", 0, "zero out the N highest-risk feature weights before evaluating -eval (0 = report only, no ablation)")
	outPath := flag.String("out", "", "optional path to write the full JSON report")
	flag.Parse()

	m, err := model.Load(*modelPath)
	if err != nil {
		return fmt.Errorf("load model: %w", err)
	}

	rep := buildReport(m, *trainDocs, *top)
	rep.ModelPath = *modelPath

	log.Printf("Model %s (version %s): %d vocabulary entries, weight norm %.4f, %d exactly-zero weights",
		*modelPath, m.Version, rep.VocabularySize, rep.WeightNorm, rep.ZeroWeights)
	for _, fs := range rep.FamilyStats {
		log.Printf("  family %-11s count=%-5d zero=%-5d mean|weight|=%.5f", fs.Family, fs.Count, fs.ZeroCount, fs.MeanAbs)
	}

	log.Printf("Top %d features by risk score (|weight| * idf — high weight, low document support):", len(rep.TopRisk))
	for _, r := range rep.TopRisk {
		df := ""
		if r.DocFrequency > 0 {
			df = fmt.Sprintf(" df~=%d", r.DocFrequency)
		}
		log.Printf("  #%-3d %-28s family=%-11s weight=%+.5f idf=%.3f risk=%.5f%s", r.Rank, r.Term, r.Family, r.Weight, r.IDF, r.RiskScore, df)
	}

	if *evalPath != "" {
		_, texts, labels, err := evalmetrics.ReadLabeledCSV(*evalPath)
		if err != nil {
			return fmt.Errorf("read eval csv: %w", err)
		}
		accuracy, precision, recall, f1 := evaluate(m, m.Weights, texts, labels)
		log.Printf("Baseline @ threshold %.9f on %s (n=%d) — accuracy: %.3f, precision: %.3f, recall: %.3f, f1: %.3f",
			m.Threshold, *evalPath, len(texts), accuracy, precision, recall, f1)

		if *ablateN > 0 {
			n := *ablateN
			if n > len(rep.TopRisk) {
				n = len(rep.TopRisk)
			}
			indices := make([]int, n)
			ablatedTerms := make([]string, n)
			for i := 0; i < n; i++ {
				indices[i] = rep.TopRisk[i].Index
				ablatedTerms[i] = rep.TopRisk[i].Term
			}
			ablatedWeights := ablate(m.Weights, indices)
			aAcc, aPrec, aRec, aF1 := evaluate(m, ablatedWeights, texts, labels)
			log.Printf("After zeroing top %d risk features (%s) — accuracy: %.3f (%+.3f), precision: %.3f (%+.3f), recall: %.3f (%+.3f), f1: %.3f (%+.3f)",
				n, strings.Join(ablatedTerms, ", "), aAcc, aAcc-accuracy, aPrec, aPrec-precision, aRec, aRec-recall, aF1, aF1-f1)
		}
	}

	if *outPath != "" {
		data, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal report: %w", err)
		}
		if err := os.WriteFile(*outPath, data, 0600); err != nil {
			return fmt.Errorf("write report: %w", err)
		}
		log.Printf("Wrote full report to %s", *outPath)
	}

	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
