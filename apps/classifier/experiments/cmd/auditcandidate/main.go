// auditcandidate reports validation errors without exposing raw corpus text.
package main

import (
	"encoding/json"
	"fmt"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/classify"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/confirmation"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/dedup"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/model"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/review"
	"log"
	"math/bits"
	"os"
	"path/filepath"
)

func main() {
	if e := run(); e != nil {
		log.Fatal(e)
	}
}
func run() error {
	c, e := confirmation.Load("data/confirmation-campaign", ".")
	if e != nil {
		return e
	}
	b, e := os.ReadFile("data/score-investigation-20261013/protocol.json")
	if e != nil {
		return e
	}
	var p struct {
		Train []string `json:"train_ids"`
		Val   []string `json:"val_ids"`
	}
	if e = json.Unmarshal(b, &p); e != nil {
		return e
	}
	reviewedLabels, e := review.LoadReviewed("data/reviewed_labels.csv")
	if e != nil {
		return e
	}
	reviewed := map[string]int{}
	for id, l := range reviewedLabels {
		reviewed[id] = l.Label
	}
	docs := map[string]string{}
	for _, d := range c.Corpus {
		docs[d.ID] = d.Text
	}
	var cls []*classify.Classifier
	for _, name := range []string{"frozen-l2-0.5-model.json", "word-l2-0.005-f1-model.json"} {
		m, e := model.Load(filepath.Join("data/vocabulary-experiment-20261013", name))
		if e != nil {
			return e
		}
		cls = append(cls, classify.New(m))
	}
	prints := make([]uint64, len(p.Train))
	for i, id := range p.Train {
		prints[i] = dedup.Fingerprint(docs[id])
	}
	var rows []map[string]interface{}
	near, errors := 0, 0
	for _, id := range p.Val {
		a, b := cls[0].Classify(docs[id]), cls[1].Classify(docs[id])
		label := reviewed[id]
		fp := dedup.Fingerprint(docs[id])
		distance := 65
		nearest := ""
		for i, f := range prints {
			if n := bits.OnesCount64(fp ^ f); n < distance {
				distance = n
				nearest = p.Train[i]
			}
		}
		if distance <= 10 {
			near++
		}
		if a.Relevant != (label == 1) {
			errors++
		}
		rows = append(rows, map[string]interface{}{"id": id, "label": label, "precision_prediction": a.Relevant, "recall_prediction": b.Relevant, "precision_score": a.Score, "recall_score": b.Score, "near_train_distance": distance, "nearest_train_id": nearest, "nearest_train_label": reviewed[nearest]})
	}
	out := "data/vocabulary-experiment-20261013/validation-audit.json"
	f, e := os.OpenFile(out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if e = enc.Encode(rows); e != nil {
		return e
	}
	fmt.Printf("Validation n=%d; near training <=10: %d; precision candidate errors=%d\n", len(rows), near, errors)
	return nil
}
