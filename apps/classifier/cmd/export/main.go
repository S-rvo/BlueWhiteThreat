// apps/classifier/cmd/export/main.go
//
// Builds a review queue for human-style relabeling: every current heuristic
// positive (to catch false positives from the keyword heuristic) plus a
// sample of heuristic negatives (to catch missed true positives), skipping
// anything already present in the persistent reviewed-labels store. Output
// is JSONL, one document per line, so it can be read and corrected in
// bounded batches without loading the whole corpus into memory/context at
// once.
//
// When a trained model.json is available, the heuristic-negative sample is
// prioritized by model confidence (highest score first) instead of random
// sampling. A heuristic negative the current model scores highly is exactly
// the case where the cheap keyword heuristic and the learned model
// disagree — reviewing those first finds mislabeled bootstrap negatives far
// faster than reviewing a random sample of the (mostly genuinely negative)
// pool.
package main

import (
	"context"
	"encoding/json"
	"log"
	"math/rand"
	"os"
	"sort"
	"strconv"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/bootstrap"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/db"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/model"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/review"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/tfidf"
	"github.com/joho/godotenv"
)

// ReviewCandidate is one line of the exported JSONL review queue.
type ReviewCandidate struct {
	ID             string  `json:"id"`
	HeuristicLabel int     `json:"heuristic_label"`
	ModelScore     float64 `json:"model_score,omitempty"`
	NeedsReview    bool    `json:"needs_review,omitempty"`
	Content        string  `json:"content"`
}

// prioritizeNeedsReview stable-sorts candidates so every one already
// flagged needs_review by the live classification pipeline (its score fell
// within classify.ReviewMargin of the model's threshold at scoring time)
// comes first, closing the loop between "the model told us it wasn't
// sure" and "a human looks at exactly those documents first". Candidates
// are otherwise left in their existing order (typically the heuristic/
// model-score prioritization already applied), since this only needs to
// promote the uncertain ones, not fully re-rank the queue.
func prioritizeNeedsReview(candidates []ReviewCandidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].NeedsReview && !candidates[j].NeedsReview
	})
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env found or unreadable, continuing with existing environment.")
	}

	outputPath := getEnvOrDefault("EXPORT_OUTPUT", "review_queue.jsonl")
	keywordsPath := getEnvOrDefault("SEED_KEYWORDS_PATH", "data/seed_keywords.json")
	reviewedPath := getEnvOrDefault("REVIEWED_LABELS_PATH", "data/reviewed_labels.csv")
	modelPath := getEnvOrDefault("MODEL_PATH", "model.json")
	negativeSample := getEnvIntOrDefault("EXPORT_NEGATIVE_SAMPLE", 400)
	seed := int64(getEnvIntOrDefault("EXPORT_SEED", 42))

	var vectorizer *tfidf.Vectorizer
	var lr *model.LogisticRegression
	if m, err := model.Load(modelPath); err != nil {
		log.Printf("no usable model at %s (%v) — falling back to random sampling of heuristic negatives", modelPath, err)
	} else {
		vectorizer = &tfidf.Vectorizer{Vocabulary: m.Vocabulary, IDF: m.IDF}
		lr = &model.LogisticRegression{Weights: m.Weights, Bias: m.Bias}
		log.Printf("loaded model %s — prioritizing heuristic negatives by model confidence", m.Version)
	}

	keywords, err := bootstrap.LoadKeywords(keywordsPath)
	if err != nil {
		log.Fatalf("failed to load seed keywords: %v", err)
	}

	reviewed, err := review.LoadReviewed(reviewedPath)
	if err != nil {
		log.Fatalf("failed to load reviewed labels: %v", err)
	}
	log.Printf("%d documents already reviewed — excluding them from the export", len(reviewed))

	ctx := context.Background()
	client, err := db.Connect(ctx)
	if err != nil {
		log.Fatalf("failed to connect to MongoDB: %v", err)
	}
	defer client.Disconnect(ctx)

	articles, err := db.FetchAllContent(ctx, client)
	if err != nil {
		log.Fatalf("failed to fetch articles: %v", err)
	}
	log.Printf("Fetched %d articles from MongoDB", len(articles))

	var positives, negatives []ReviewCandidate
	for _, article := range articles {
		if article.Content == "" || bootstrap.IsLinkDirectory(article.Content) {
			continue
		}
		id := review.IDString(article.ID)
		if _, ok := reviewed[id]; ok {
			continue
		}
		label := bootstrap.LabelByKeywords(article.Content, keywords)
		candidate := ReviewCandidate{ID: id, HeuristicLabel: label, Content: article.Content}
		if article.Relevance != nil {
			candidate.NeedsReview = article.Relevance.NeedsReview
		}
		if lr != nil {
			candidate.ModelScore = lr.Predict(vectorizer.Transform(article.Content))
		}
		if label == 1 {
			positives = append(positives, candidate)
		} else {
			negatives = append(negatives, candidate)
		}
	}

	// Sort by id first so the sample is deterministic regardless of the
	// (unordered) order MongoDB returns documents in.
	sort.Slice(negatives, func(i, j int) bool { return negatives[i].ID < negatives[j].ID })
	sort.Slice(positives, func(i, j int) bool { return positives[i].ID < positives[j].ID })

	if lr != nil {
		// Highest model score first: a heuristic negative the model is most
		// confident is actually relevant is the highest-value thing to review.
		sort.SliceStable(negatives, func(i, j int) bool { return negatives[i].ModelScore > negatives[j].ModelScore })
		// Lowest model score first: a heuristic positive the model is most
		// confident is actually not relevant is the mirror-image disagreement.
		sort.SliceStable(positives, func(i, j int) bool { return positives[i].ModelScore < positives[j].ModelScore })
	} else {
		rng := rand.New(rand.NewSource(seed))
		rng.Shuffle(len(negatives), func(i, j int) { negatives[i], negatives[j] = negatives[j], negatives[i] })
	}
	if negativeSample < len(negatives) {
		negatives = negatives[:negativeSample]
	}

	// Final pass: whatever order the heuristic/model-score sorting above
	// produced, documents the live pipeline already flagged needs_review
	// (its score landed within classify.ReviewMargin of the threshold) jump
	// to the front — the model telling us it's unsure is a stronger review
	// signal than the heuristic/model disagreement ordering above.
	prioritizeNeedsReview(positives)
	prioritizeNeedsReview(negatives)

	file, err := os.Create(outputPath)
	if err != nil {
		log.Fatalf("failed to create output file: %v", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	written := 0
	for _, c := range append(positives, negatives...) {
		if err := encoder.Encode(c); err != nil {
			log.Printf("skipping candidate %s due to encode error: %v", c.ID, err)
			continue
		}
		written++
	}

	log.Printf("Wrote %d review candidates to %s (%d heuristic positives, %d sampled heuristic negatives)",
		written, outputPath, len(positives), len(negatives))
}

func getEnvOrDefault(key, defaultValue string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultValue
}

func getEnvIntOrDefault(key string, defaultValue int) int {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		if n, err := strconv.Atoi(val); err == nil {
			return n
		}
	}
	return defaultValue
}
