// apps/classifier/cmd/main/main.go
package main

import (
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/bootstrap"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/classify"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/db"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/model"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/mongo"
)

var defaultIntervalMinutes = 10

const legacyPipelineSourceType = "darkweb_page"

func main() {
	once := flag.Bool("once", false, "run one classification cycle and exit")
	flag.Parse()
	if err := godotenv.Load(); err != nil {
		log.Println("No .env found or unreadable, continuing with existing environment.")
	}

	if i, err := strconv.Atoi(os.Getenv("CLASSIFIER_INTERVAL_MIN")); err == nil && i > 0 {
		defaultIntervalMinutes = i
	}

	modelPath := getEnvOrDefault("MODEL_PATH", "model.json")
	thresholdOverride := os.Getenv("RELEVANCE_THRESHOLD")
	classifier, err := loadClassifier(modelPath, thresholdOverride)
	if err != nil {
		log.Fatalf("failed to load model from %s: %v (run cmd/train first)", modelPath, err)
	}

	log.Printf("Classifier service started (model version %s, interval %d min)", classifier.Version(), defaultIntervalMinutes)
	if err := runOnce(classifier); err != nil && *once {
		log.Fatal(err)
	}
	if *once {
		return
	}

	ticker := time.NewTicker(time.Duration(defaultIntervalMinutes) * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		log.Printf("Scheduled classification run at %s", time.Now().Format("15:04:05"))
		updatedClassifier, err := loadClassifier(modelPath, thresholdOverride)
		if err != nil {
			log.Printf("failed to reload model from %s; skipping this cycle: %v", modelPath, err)
			continue
		}
		classifier = updatedClassifier
		log.Printf("Using model version %s", classifier.Version())
		runOnce(classifier)
	}
}

func loadClassifier(modelPath, thresholdOverride string) (*classify.Classifier, error) {
	m, err := model.Load(modelPath)
	if err != nil {
		return nil, err
	}
	if m.Version == "" {
		modelBytes, err := os.ReadFile(modelPath)
		if err != nil {
			return nil, fmt.Errorf("read model for fallback version: %w", err)
		}
		digest := sha256.Sum256(modelBytes)
		m.Version = fmt.Sprintf("legacy-%x", digest[:8])
	}
	applyThresholdOverride(m, thresholdOverride)
	return classify.New(m), nil
}

// applyThresholdOverride updates the decision threshold and derives a distinct
// inference version when the effective threshold differs from the model file.
// The threshold changes labels even when model weights stay the same, so it
// must trigger a rescore of existing articles.
func applyThresholdOverride(m *model.Model, raw string) bool {
	if raw == "" {
		return false
	}
	threshold, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(threshold) || math.IsInf(threshold, 0) || threshold < 0 || threshold > 1 {
		log.Printf("ignoring invalid RELEVANCE_THRESHOLD %q; expected a number in [0,1]", raw)
		return false
	}
	if threshold == m.Threshold {
		return false
	}
	m.Threshold = threshold
	m.Version = fmt.Sprintf("%s-threshold-%s", m.Version, strconv.FormatFloat(threshold, 'g', -1, 64))
	return true
}

// runOnce classifies every pending article in bounded batches. Each batch has
// its own deadline, so a large backlog (new model or policy version) is worked
// through over one run instead of timing out on a single unbounded fetch.
func runOnce(classifier *classify.Classifier) error {
	connectCtx, cancelConnect := context.WithTimeout(context.Background(), 30*time.Second)
	client, err := db.Connect(connectCtx)
	cancelConnect()
	if err != nil {
		log.Printf("failed to connect to MongoDB, will retry next tick: %v", err)
		return err
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.Disconnect(ctx)
	}()

	var (
		writeErr error
		after    interface{}
		total    int
	)
	for {
		n, last, err := classifyBatch(classifier, client, after)
		total += n
		if err != nil {
			log.Printf("failed to fetch articles needing classification, will retry next tick: %v", err)
			return err
		}
		if last.err != nil {
			writeErr = last.err
		}
		if n < int(db.ClassificationBatchSize) {
			break
		}
		after = last.id
	}
	log.Printf("Processed %d articles needing classification by model %s", total, classifier.Version())
	return writeErr
}

type batchCursor struct {
	id  interface{}
	err error
}

// classifyBatch fetches and scores one batch after the given _id. It returns
// the batch size, the last _id seen and the last write error, if any.
func classifyBatch(classifier *classify.Classifier, client *mongo.Client, after interface{}) (int, batchCursor, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	articles, err := db.FetchNeedingClassification(ctx, client, classifier.Version(), after, db.ClassificationBatchSize)
	if err != nil {
		return 0, batchCursor{}, err
	}
	var cur batchCursor
	for _, article := range articles {
		cur.id = article.ID
		relevance, skip := classifyArticleSource(classifier, article.Content, article.SourceType)
		if skip {
			log.Printf("skipping article %v: empty content", article.ID)
			continue
		}
		if err := db.UpdateRelevance(ctx, client, article, relevance); err != nil {
			log.Printf("failed to update relevance for article %v: %v", article.ID, err)
			cur.err = err
		}
	}
	return len(articles), cur, nil
}

// classifyArticle decides the relevance record for one article's content.
// Empty content can't be scored and is skipped entirely (left for a later
// pass). Link-directory pages (bootstrap.IsLinkDirectory) and gated
// login-wall pages (bootstrap.IsGatedLoginPage) are excluded from training
// data by cmd/bootstrap and cmd/export, so the live classifier never saw one
// as a labeled example — scoring it here would apply a model to data it was
// never evaluated against, so it's instead recorded directly as not
// relevant, keeping training and inference consistent.
func classifyArticle(classifier *classify.Classifier, content string) (relevance db.Relevance, skip bool) {
	return classifyArticleSource(classifier, content, "")
}

func classifyArticleSource(classifier *classify.Classifier, content, source string) (relevance db.Relevance, skip bool) {
	if content == "" {
		return db.Relevance{}, true
	}

	if bootstrap.IsLinkDirectory(content) || bootstrap.IsGatedLoginPage(content) {
		return db.Relevance{Relevant: false, Score: 0, ModelVersion: classifier.Version(), PolicyVersion: db.ClassificationPolicyVersion}, false
	}

	// Documents scraped before source_type was introduced all came from the
	// dark-web page pipeline. Keep their inference policy aligned with newly
	// scraped documents without rewriting historical MongoDB records.
	if source == "" {
		source = legacyPipelineSourceType
	}
	result := classifier.ClassifySource(content, source)
	return db.Relevance{
		Relevant:        result.Relevant,
		Score:           result.Score,
		CalibratedScore: result.CalibratedScore,
		ModelVersion:    result.ModelVersion,
		PolicyVersion:   db.ClassificationPolicyVersion,
		NeedsReview:     result.NeedsReview,
		ReviewReason:    result.ReviewReason,
	}, false
}

func getEnvOrDefault(key, defaultValue string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultValue
}
