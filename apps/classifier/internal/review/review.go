// apps/classifier/internal/review/review.go
package review

import (
	"encoding/csv"
	"fmt"
	"os"

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

// LoadReviewed reads a persisted reviewed-labels store (id,label CSV) into a
// map. Missing file is not an error — it just means nothing has been
// reviewed yet.
func LoadReviewed(path string) (map[string]int, error) {
	reviewed := map[string]int{}

	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return reviewed, nil
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
	for _, row := range rows[1:] { // skip header
		if len(row) != 2 {
			continue
		}
		var label int
		if _, err := fmt.Sscanf(row[1], "%d", &label); err != nil {
			continue
		}
		reviewed[row[0]] = label
	}
	return reviewed, nil
}
