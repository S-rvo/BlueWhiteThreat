// apps/classifier/cmd/bootstrap/main.go
package main

import (
	"context"
	"encoding/csv"
	"log"
	"os"
	"strings"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/bootstrap"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/db"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/review"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env found or unreadable, continuing with existing environment.")
	}

	outputPath := getEnvOrDefault("BOOTSTRAP_OUTPUT", "training_data.csv")
	keywordsPath := getEnvOrDefault("SEED_KEYWORDS_PATH", "data/seed_keywords.json")
	reviewedPath := getEnvOrDefault("REVIEWED_LABELS_PATH", "data/reviewed_labels.csv")
	excludedPath := getEnvOrDefault("EXCLUDED_IDS_PATH", "data/excluded_ids.csv")
	themeNegativesPath := getEnvOrDefault("THEME_NEGATIVES_PATH", "data/theme_negatives.csv")
	inScopeThemes := parseThemeSet(os.Getenv("INSCOPE_THEMES"))

	keywords, err := bootstrap.LoadKeywords(keywordsPath)
	if err != nil {
		log.Fatalf("failed to load seed keywords: %v", err)
	}

	// reviewed holds reviewed labels keyed by stable Mongo _id. Unlike
	// training_data.csv, this file is never regenerated — it survives every
	// bootstrap cycle and overrides the noisy keyword heuristic below.
	reviewed, err := review.LoadReviewed(reviewedPath)
	if err != nil {
		log.Fatalf("failed to load reviewed labels: %v", err)
	}
	log.Printf("Loaded %d reviewed label overrides from %s", len(reviewed), reviewedPath)

	// excluded holds documents a human reviewer found to be CSAM or
	// CSAM-adjacent. These must never enter training_data.csv under any
	// label — excluding them is not the same as labeling them negative.
	excluded, err := review.LoadExcluded(excludedPath)
	if err != nil {
		log.Fatalf("failed to load excluded ids: %v", err)
	}
	log.Printf("Loaded %d excluded document ids from %s", len(excluded), excludedPath)

	// themeNegatives maps a document id to the out-of-CTI-scope theme it belongs
	// to (e.g. "geopolitics"). Such a document is forced to label=0 — but only
	// while its theme is out of scope. Declaring the theme in INSCOPE_THEMES
	// (comma-separated) lifts the override and lets the document revert to its
	// natural label, so a theme can be brought into scope later without editing
	// the store. An explicit reviewed label always wins over this policy default.
	themeNegatives, err := review.LoadThemeNegatives(themeNegativesPath)
	if err != nil {
		log.Fatalf("failed to load theme negatives: %v", err)
	}
	log.Printf("Loaded %d theme-negative ids from %s (in-scope themes: %v)", len(themeNegatives), themeNegativesPath, inScopeThemesList(inScopeThemes))

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

	if err := writer.Write([]string{"id", "text", "label"}); err != nil {
		log.Fatalf("failed to write CSV header: %v", err)
	}

	written, skippedDirectories, skippedGatedLogin, skippedExcluded, overridden, themeForced := 0, 0, 0, 0, 0, 0
	for _, article := range articles {
		if article.Content == "" {
			continue
		}
		if bootstrap.IsLinkDirectory(article.Content) {
			skippedDirectories++
			continue
		}
		if bootstrap.IsGatedLoginPage(article.Content) {
			skippedGatedLogin++
			continue
		}
		id := review.IDString(article.ID)
		if excluded[id] {
			skippedExcluded++
			continue
		}
		label := bootstrap.LabelByKeywords(article.Content, keywords)
		if theme, ok := themeNegatives[id]; ok && !inScopeThemes[strings.ToLower(theme)] {
			label = 0
			themeForced++
		}
		if reviewedLabel, ok := reviewed[id]; ok {
			label = reviewedLabel.Label
			overridden++
		}
		if err := writer.Write([]string{id, article.Content, itoa(label)}); err != nil {
			log.Printf("skipping article due to write error: %v", err)
			continue
		}
		written++
	}

	log.Printf("Wrote %d labeled candidates to %s (skipped %d link-directory pages, %d gated login pages, %d excluded, %d forced negative by out-of-scope theme, %d overridden by human review)", written, outputPath, skippedDirectories, skippedGatedLogin, skippedExcluded, themeForced, overridden)
}

// parseThemeSet turns a comma-separated INSCOPE_THEMES value into a lookup set,
// trimming whitespace and lowercasing so "Geopolitics, espionage" matches the
// themes stored in theme_negatives.csv. An empty value yields an empty set, so
// by default every theme listed in the store is treated as out of scope.
func parseThemeSet(raw string) map[string]bool {
	set := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		if t := strings.ToLower(strings.TrimSpace(part)); t != "" {
			set[t] = true
		}
	}
	return set
}

func inScopeThemesList(set map[string]bool) []string {
	themes := make([]string, 0, len(set))
	for t := range set {
		themes = append(themes, t)
	}
	return themes
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
