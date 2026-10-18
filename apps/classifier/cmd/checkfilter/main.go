package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/db"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/review"
	"github.com/joho/godotenv"
)

var suspectTerms = []string{
	"loliporn", "pedo porn", "pedomom", "kidsparty", "diamond porn links", "child porno",
}

func main() {
	godotenv.Load()
	ctx := context.Background()
	client, err := db.Connect(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Disconnect(ctx)
	articles, err := db.FetchAllContent(ctx, client)
	if err != nil {
		log.Fatal(err)
	}

	// excluded holds documents a human already reviewed and confirmed as
	// CSAM/CSAM-adjacent, so they don't keep halting every retrain cycle —
	// only a *new* match (not yet in this list) should require fresh human
	// review.
	excludedPath := getEnvOrDefault("EXCLUDED_IDS_PATH", "data/excluded_ids.csv")
	excluded, err := review.LoadExcluded(excludedPath)
	if err != nil {
		log.Fatal(err)
	}

	count := 0
	for _, a := range articles {
		if excluded[review.IDString(a.ID)] {
			continue
		}
		low := strings.ToLower(a.Content)
		for _, term := range suspectTerms {
			if strings.Contains(low, term) {
				count++
				fmt.Printf("MATCH term=%q id=%v\n", term, a.ID)
				break
			}
		}
	}
	fmt.Println("total matching docs:", count)
}

func getEnvOrDefault(key, defaultValue string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultValue
}
