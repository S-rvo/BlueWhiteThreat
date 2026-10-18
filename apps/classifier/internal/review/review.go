// apps/classifier/internal/review/review.go
package review

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// IDString renders a Mongo document id as a stable, comparable string key,
// independent of whether the driver decoded it as primitive.ObjectID or
// something else. Used as the merge key between heuristic labels (recomputed
// from scratch every bootstrap cycle) and human-reviewed corrections (which
// must persist across cycles).
func IDString(id interface{}) string {
	if oid, ok := id.(primitive.ObjectID); ok {
		return oid.Hex()
	}
	return fmt.Sprintf("%v", id)
}

// Label preserves the provenance of a reviewed correction.
type Label struct {
	Label  int
	Source string
}

const (
	SourceHuman = "human"
	SourceLLM   = "llm_reviewed"
)

// LoadReviewed accepts id,label[,source] CSV. Legacy rows are human reviews.
// A missing or empty store means nothing has been reviewed yet.
func LoadReviewed(path string) (map[string]Label, error) {
	reviewed := map[string]Label{}
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return reviewed, nil
	}
	if err != nil {
		return nil, fmt.Errorf("review: open %s: %w", path, err)
	}
	defer file.Close()
	rows, err := csv.NewReader(file).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("review: parse %s: %w", path, err)
	}
	if len(rows) == 0 {
		return reviewed, nil
	}
	header := rows[0]
	if (len(header) != 2 && len(header) != 3) || header[0] != "id" || header[1] != "label" || (len(header) == 3 && header[2] != "source") {
		return nil, fmt.Errorf("review: invalid header in %s", path)
	}
	for i, row := range rows[1:] {
		label, err := strconv.Atoi(row[1])
		if err != nil || (label != 0 && label != 1) || row[0] == "" {
			return nil, fmt.Errorf("review: invalid label or id in %s row %d", path, i+2)
		}
		source := SourceHuman
		if len(row) == 3 {
			source = row[2]
		}
		if source != SourceHuman && source != SourceLLM {
			return nil, fmt.Errorf("review: invalid source %q in %s row %d", source, path, i+2)
		}
		reviewed[row[0]] = Label{Label: label, Source: source}
	}
	return reviewed, nil
}

// LoadExcluded reads a persisted exclusion store (id,reason CSV) into a set.
// Excluded documents (e.g. CSAM found during human review) must never be
// written into training_data.csv or review_queue.jsonl again, regardless of
// label — unlike a reviewed label, exclusion isn't "this is a negative
// example", it's "this document must not be used at all". Missing file is
// not an error — it just means nothing has been excluded yet.
func LoadExcluded(path string) (map[string]bool, error) {
	excluded := map[string]bool{}

	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return excluded, nil
	}
	if err != nil {
		return nil, fmt.Errorf("review: open %s: %w", path, err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("review: parse %s: %w", path, err)
	}
	if len(rows) == 0 || len(rows[0]) != 2 || rows[0][0] != "id" || rows[0][1] != "reason" {
		return nil, fmt.Errorf("review: %s must have header id,reason", path)
	}
	for i, row := range rows[1:] {
		if len(row) != 2 || row[0] == "" {
			return nil, fmt.Errorf("review: invalid excluded id in %s row %d", path, i+2)
		}
		excluded[row[0]] = true
	}
	return excluded, nil
}
