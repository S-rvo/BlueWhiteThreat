// apps/classifier/cmd/bootstrap/main.go
package main

import (
	"context"
	"encoding/csv"
	"log"
	"os"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/bootstrap"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/db"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env found or unreadable, continuing with existing environment.")
	}

	outputPath := getEnvOrDefault("BOOTSTRAP_OUTPUT", "training_data.csv")
	keywordsPath := getEnvOrDefault("SEED_KEYWORDS_PATH", "data/seed_keywords.json")

	keywords, err := bootstrap.LoadKeywords(keywordsPath)
	if err != nil {
		log.Fatalf("failed to load seed keywords: %v", err)
	}

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

	file, err := os.Create(outputPath)
	if err != nil {
		log.Fatalf("failed to create output file: %v", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	if err := writer.Write([]string{"text", "label"}); err != nil {
		log.Fatalf("failed to write CSV header: %v", err)
	}

	written := 0
	for _, article := range articles {
		if article.Content == "" {
			continue
		}
		label := bootstrap.LabelByKeywords(article.Content, keywords)
		if err := writer.Write([]string{article.Content, itoa(label)}); err != nil {
			log.Printf("skipping article due to write error: %v", err)
			continue
		}
		written++
	}

	log.Printf("Wrote %d labeled candidates to %s — review and correct labels by hand before running cmd/train.", written, outputPath)
}

func getEnvOrDefault(key, defaultValue string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultValue
}

func itoa(i int) string {
	if i == 1 {
		return "1"
	}
	return "0"
}
