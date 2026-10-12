// apps/classifier/cmd/applyreview/main.go
//
// Merges a batch of review decisions (JSONL: {"id":"...","label":0|1} per
// line) into the persistent reviewed-labels store at data/reviewed_labels.csv,
// keyed by document id. Safe to run repeatedly across many small batches —
// existing entries are overwritten by newer decisions for the same id,
// everything else is kept.
package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"log"
	"os"
	"sort"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/review"
)

type decision struct {
	ID    string `json:"id"`
	Label int    `json:"label"`
}

func main() {
	decisionsPath := getEnvOrDefault("DECISIONS_PATH", "")
	reviewedPath := getEnvOrDefault("REVIEWED_LABELS_PATH", "data/reviewed_labels.csv")
	if decisionsPath == "" {
		log.Fatal("DECISIONS_PATH env var is required")
	}

	existing, err := review.LoadReviewed(reviewedPath)
	if err != nil {
		log.Fatalf("failed to load existing reviewed labels: %v", err)
	}
	log.Printf("Loaded %d existing reviewed labels from %s", len(existing), reviewedPath)

	file, err := os.Open(decisionsPath)
	if err != nil {
		log.Fatalf("failed to open decisions file: %v", err)
	}
	defer file.Close()

	applied := 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var d decision
		if err := json.Unmarshal([]byte(line), &d); err != nil {
			log.Printf("skipping malformed decision line: %v", err)
			continue
		}
		if d.Label != 0 && d.Label != 1 {
			log.Printf("skipping decision %s with invalid label %d", d.ID, d.Label)
			continue
		}
		existing[d.ID] = d.Label
		applied++
	}
	if err := scanner.Err(); err != nil {
		log.Fatalf("failed reading decisions file: %v", err)
	}

	ids := make([]string, 0, len(existing))
	for id := range existing {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	out, err := os.Create(reviewedPath)
	if err != nil {
		log.Fatalf("failed to create %s: %v", reviewedPath, err)
	}
	defer out.Close()

	writer := csv.NewWriter(out)
	defer writer.Flush()
	if err := writer.Write([]string{"id", "label"}); err != nil {
		log.Fatalf("failed to write header: %v", err)
	}
	for _, id := range ids {
		if err := writer.Write([]string{id, itoa(existing[id])}); err != nil {
			log.Fatalf("failed to write row: %v", err)
		}
	}

	log.Printf("Applied %d decisions; %s now holds %d reviewed labels total", applied, reviewedPath, len(existing))
}

func itoa(i int) string {
	if i == 1 {
		return "1"
	}
	return "0"
}

func getEnvOrDefault(key, defaultValue string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultValue
}
