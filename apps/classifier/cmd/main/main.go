// apps/classifier/cmd/main/main.go
package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/bootstrap"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/classify"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/db"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/model"
	"github.com/joho/godotenv"
)

var defaultIntervalMinutes = 10

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env found or unreadable, continuing with existing environment.")
	}

	if i, err := strconv.Atoi(os.Getenv("CLASSIFIER_INTERVAL_MIN")); err == nil && i > 0 {
		defaultIntervalMinutes = i
	}

	modelPath := getEnvOrDefault("MODEL_PATH", "model.json")
	m, err := model.Load(modelPath)
	if err != nil {
		log.Fatalf("failed to load model from %s: %v (run cmd/train first)", modelPath, err)
	}

	if override := os.Getenv("RELEVANCE_THRESHOLD"); override != "" {
		if v, err := strconv.ParseFloat(override, 64); err == nil {
			m.Threshold = v
		}
	}

	classifier := classify.New(m)

	log.Printf("Classifier service started (model version %s, interval %d min)", m.Version, defaultIntervalMinutes)
	runOnce(classifier)

	ticker := time.NewTicker(time.Duration(defaultIntervalMinutes) * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		log.Printf("Scheduled classification run at %s", time.Now().Format("15:04:05"))
		runOnce(classifier)
	}
}

func runOnce(classifier *classify.Classifier) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := db.Connect(ctx)
	if err != nil {
		log.Printf("failed to connect to MongoDB, will retry next tick: %v", err)
		return
	}
	defer client.Disconnect(ctx)

	articles, err := db.FetchUnclassified(ctx, client)
	if err != nil {
		log.Printf("failed to fetch unclassified articles, will retry next tick: %v", err)
		return
	}
	log.Printf("Found %d unclassified articles", len(articles))

	for _, article := range articles {
		relevance, skip := classifyArticle(classifier, article.Content)
		if skip {
			log.Printf("skipping article %v: empty content", article.ID)
			continue
		}

		if err := db.UpdateRelevance(ctx, client, article.ID, relevance); err != nil {
			log.Printf("failed to update relevance for article %v: %v", article.ID, err)
			continue
		}
	}
}

// classifyArticle decides the relevance record for one article's content.
// Empty content can't be scored and is skipped entirely (left for a later
// pass). Link-directory pages (bootstrap.IsLinkDirectory) are excluded from
// training data by cmd/bootstrap and cmd/export, so the live classifier
// never saw one as a labeled example — scoring it here would apply a model
// to data it was never evaluated against, so it's instead recorded directly
// as not relevant, keeping training and inference consistent.
func classifyArticle(classifier *classify.Classifier, content string) (relevance db.Relevance, skip bool) {
	if content == "" {
		return db.Relevance{}, true
	}

	if bootstrap.IsLinkDirectory(content) {
		return db.Relevance{Relevant: false, Score: 0, ModelVersion: classifier.Version()}, false
	}

	result := classifier.Classify(content)
	return db.Relevance{
		Relevant:        result.Relevant,
		Score:           result.Score,
		CalibratedScore: result.CalibratedScore,
		ModelVersion:    result.ModelVersion,
		NeedsReview:     result.NeedsReview,
	}, false
}

func getEnvOrDefault(key, defaultValue string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultValue
}
