// apps/classifier/internal/classify/classify.go
package classify

import (
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/model"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/tfidf"
)

// Result is the outcome of scoring one document.
type Result struct {
	Relevant     bool
	Score        float64
	ModelVersion string
}

// Classifier combines a fitted vectorizer and a trained logistic regression
// into a single Classify(text) call, so callers never need to know about
// TF-IDF or model internals.
type Classifier struct {
	vectorizer *tfidf.Vectorizer
	logreg     *model.LogisticRegression
	threshold  float64
	version    string
}

// New builds a ready-to-use Classifier from a loaded model.Model.
func New(m *model.Model) *Classifier {
	return &Classifier{
		vectorizer: &tfidf.Vectorizer{Vocabulary: m.Vocabulary, IDF: m.IDF},
		logreg:     &model.LogisticRegression{Weights: m.Weights, Bias: m.Bias},
		threshold:  m.Threshold,
		version:    m.Version,
	}
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
// its score falls back to sigmoid(Bias).
func (c *Classifier) Classify(text string) Result {
	vector := c.vectorizer.Transform(text)
	score := c.logreg.Predict(vector)
	return Result{
		Relevant:     score >= c.threshold,
		Score:        score,
		ModelVersion: c.version,
	}
}
