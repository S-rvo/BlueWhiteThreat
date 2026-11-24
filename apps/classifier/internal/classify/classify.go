// apps/classifier/internal/classify/classify.go
package classify

import (
	"math"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/model"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/tfidf"
)

// ReviewMargin defines how close a score must be to the model's threshold
// to be flagged NeedsReview: scores within [threshold-ReviewMargin,
// threshold+ReviewMargin] are borderline calls the model isn't confident
// about, so a human should check them rather than trusting the automatic
// Relevant verdict either way. Documents without any active vocabulary feature
// also need review, independently of the score's distance from the threshold.
const ReviewMargin = 0.1

const (
	ReviewNoFeatures    = "no_active_features"
	ReviewNearThreshold = "near_threshold"
)

// Result is the outcome of scoring one document.
type Result struct {
	Relevant bool
	Score    float64
	// CalibratedScore is Score passed through the model's Platt-scaling
	// calibrator, so it reflects the true observed positive rate at that
	// confidence level instead of the raw (and, under balanced-weighted
	// training, systematically overconfident) sigmoid output. Relevant and
	// NeedsReview are still decided on the raw Score against threshold,
	// since that's what the threshold was picked against — CalibratedScore
	// is for anything that treats Score as an actual probability.
	CalibratedScore float64
	ModelVersion    string
	NeedsReview     bool
	// ReviewReason is empty when no review is requested. No-feature evidence
	// takes precedence over a score near the threshold, which may reflect bias only.
	ReviewReason string
}

// Classifier combines a fitted vectorizer and a trained logistic regression
// into a single Classify(text) call, so callers never need to know about
// TF-IDF or model internals.
type Classifier struct {
	vectorizer        *tfidf.Vectorizer
	logreg            *model.LogisticRegression
	threshold         float64
	sourceThresholds  map[string]float64
	sourceHeads       map[string]*model.LogisticRegression
	sourceCalibration map[string]model.Calibrator
	calibration       model.Calibrator
	version           string
}

// New builds a ready-to-use Classifier from a loaded model.Model.
func New(m *model.Model) *Classifier {
	c := &Classifier{
		vectorizer:       &tfidf.Vectorizer{Vocabulary: m.Vocabulary, IDF: m.IDF, LegacyCharNgrams: m.CharNgrams, LegacySublinearTF: m.SublinearTF},
		logreg:           &model.LogisticRegression{Weights: m.Weights, Bias: m.Bias},
		threshold:        m.Threshold,
		sourceThresholds: m.SourceThresholds,
		calibration:      m.Calibration,
		version:          m.Version,
	}
	if len(m.SourceHeads) > 0 {
		c.sourceHeads = make(map[string]*model.LogisticRegression, len(m.SourceHeads))
		c.sourceCalibration = make(map[string]model.Calibrator, len(m.SourceHeads))
		for source, head := range m.SourceHeads {
			c.sourceHeads[source] = &model.LogisticRegression{Weights: head.Weights, Bias: head.Bias}
			c.sourceCalibration[source] = head.Calibration
		}
	}
	return c
}

// Version reports the model version this Classifier was built from, so
// callers can attach it to records they write without re-deriving it.
func (c *Classifier) Version() string {
	return c.version
}

// Classify scores text for relevance: it vectorizes text with the
// Classifier's TF-IDF vectorizer, runs the resulting vector through the
// logistic regression model, and reports Relevant as true when the
// predicted score is >= the model's threshold. A document with no tokens
// in the vocabulary (or no tokens at all) yields an all-zero vector, so
// its score falls back to sigmoid(Bias) and it is flagged for review because
// the verdict then contains no document-specific feature evidence.
func (c *Classifier) Classify(text string) Result {
	return c.ClassifySource(text, "")
}

// ClassifySource uses a preselected source-specific threshold when present.
// The raw score and calibration are identical to Classify; only the frozen
// decision boundary changes. Unknown sources retain the global fallback.
func (c *Classifier) ClassifySource(text, source string) Result {
	vector := c.vectorizer.Transform(text)
	hasFeatures := false
	for _, value := range vector {
		if value != 0 {
			hasFeatures = true
			break
		}
	}
	head := c.logreg
	calibration := c.calibration
	validSourceHead := false
	if sourceHead, ok := c.sourceHeads[source]; ok && len(sourceHead.Weights) == len(vector) {
		head = sourceHead
		calibration = c.sourceCalibration[source]
		validSourceHead = true
	}
	score := head.Predict(vector)
	threshold := c.threshold
	_, sourceHasHead := c.sourceHeads[source]
	if sourceThreshold, ok := c.sourceThresholds[source]; ok && (!sourceHasHead || validSourceHead) {
		threshold = sourceThreshold
	}
	reviewReason := ""
	if !hasFeatures {
		reviewReason = ReviewNoFeatures
	} else if math.Abs(score-threshold) <= ReviewMargin {
		reviewReason = ReviewNearThreshold
	}
	return Result{
		Relevant:        score >= threshold,
		Score:           score,
		CalibratedScore: calibration.Apply(score),
		ModelVersion:    c.version,
		NeedsReview:     reviewReason != "",
		ReviewReason:    reviewReason,
	}
}
