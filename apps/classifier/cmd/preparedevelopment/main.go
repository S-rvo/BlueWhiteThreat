// preparedevelopment freezes a development-only corpus; never reads the reserve.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/dedup"
	"os"
)

func main() {
	input := flag.String("input", "", "development JSONL")
	consensus := flag.String("consensus", "", "original consensus")
	overlay := flag.String("overlay", "", "new automatic consensus")
	out := flag.String("out", "", "new output directory")
	flag.Parse()
	if err := run(*input, *consensus, *overlay, *out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(input, consensus, overlay, out string) error {
	type decision struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Label  *int   `json:"label"`
		Hash   string `json:"text_sha256"`
	}
	type batch struct {
		Documents []decision `json:"documents"`
	}
	labels := map[string]int{}
	hashes := map[string]string{}
	for _, path := range []string{consensus, overlay} {
		raw, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		var b batch
		if e = json.Unmarshal(raw, &b); e != nil {
			return e
		}
		for _, d := range b.Documents {
			if d.Status == "consensus" && d.Label != nil {
				labels[d.ID] = *d.Label
				hashes[d.ID] = d.Hash
			}
		}
	}
	f, e := os.Open(input)
	if e != nil {
		return e
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	rows := map[string]map[string]any{}
	var ids, texts []string
	var y []float64
	for dec.More() {
		var r map[string]any
		if e = dec.Decode(&r); e != nil {
			return e
		}
		id := r["id"].(string)
		l, ok := labels[id]
		if !ok {
			continue
		}
		t := r["text"].(string)
		if fmt.Sprintf("%x", sha256.Sum256([]byte(t))) != hashes[id] {
			return fmt.Errorf("text hash mismatch: %s", id)
		}
		r["label"] = l
		rows[id] = r
		ids = append(ids, id)
		texts = append(texts, t)
		y = append(y, float64(l))
	}
	groups, exact := dedup.GroupByNormalizedText(ids, texts, y)
	merged, near, diag := dedup.MergeContentDuplicatesWithDiagnostics(groups)
	if e = os.Mkdir(out, 0700); e != nil {
		return e
	}
	write := func(name string, v any) error {
		b, e := json.MarshalIndent(v, "", "  ")
		if e != nil {
			return e
		}
		return os.WriteFile(out+"/"+name, append(b, '\n'), 0600)
	}
	clean, e := os.OpenFile(out+"/development.jsonl", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer clean.Close()
	enc := json.NewEncoder(clean)
	n := 0
	for _, g := range merged {
		for _, id := range g.IDs {
			r := rows[id]
			r["content_component"] = "content:" + g.IDs[0]
			if e = enc.Encode(r); e != nil {
				return e
			}
			n++
		}
	}
	conflicts := append(exact, near...)
	excluded := 0
	var excludedIDs []string
	for _, c := range conflicts {
		excluded += len(c.IDs)
		excludedIDs = append(excludedIDs, c.IDs...)
	}
	if e = write("excluded-ids.json", excludedIDs); e != nil {
		return e
	}
	return write("preparation.json", map[string]any{"input_consensus": len(ids), "retained": n, "excluded": excluded, "conflict_components": len(conflicts), "diagnostics": diag, "reserve_read": false, "automatic_labels_not_human_truth": true})
}
