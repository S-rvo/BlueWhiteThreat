// evaldocuments emits fingerprints or inference scores for a private JSONL batch.
// It never reads labels, calibrates a threshold, or writes to production.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/classify"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/dedup"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/model"
	"os"
)

func main() {
	mode := flag.String("mode", "fingerprints", "fingerprints or scores")
	input := flag.String("input", "", "private JSONL input")
	mp := flag.String("model", "model.json", "frozen model for scores")
	flag.Parse()
	if err := run(*mode, *input, *mp); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(mode, input, mp string) error {
	if mode != "fingerprints" && mode != "scores" {
		return fmt.Errorf("invalid mode")
	}
	var c *classify.Classifier
	if mode == "scores" {
		m, err := model.Load(mp)
		if err != nil {
			return err
		}
		c = classify.New(m)
	}
	f, err := os.Open(input)
	if err != nil {
		return err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 4096), 2*1024*1024)
	enc := json.NewEncoder(os.Stdout)
	seen := map[string]bool{}
	for s.Scan() {
		var d struct {
			ID     string `json:"id"`
			Text   string `json:"text"`
			Source string `json:"source"`
		}
		if err = json.Unmarshal(s.Bytes(), &d); err != nil {
			return err
		}
		if d.ID == "" || seen[d.ID] {
			return fmt.Errorf("empty or duplicate id")
		}
		seen[d.ID] = true
		var out any
		if mode == "fingerprints" {
			out = map[string]any{"id": d.ID, "fingerprint": fmt.Sprintf("%016x", dedup.Fingerprint(d.Text)), "normalized": dedup.Normalize(d.Text)}
		} else {
			v := c.ClassifySource(d.Text, d.Source)
			out = map[string]any{"id": d.ID, "score": v.Score, "relevant": v.Relevant}
		}
		if err = enc.Encode(out); err != nil {
			return err
		}
	}
	return s.Err()
}
