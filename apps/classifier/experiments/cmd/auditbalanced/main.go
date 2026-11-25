// auditbalanced measures leakage with the exact production dedup algorithms.
package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"math/bits"
	"os"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/dedup"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/overlap"
)

type doc struct{ ID, Text string }

func readCSV(path string) ([]doc, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	rows, e := csv.NewReader(f).ReadAll()
	if e != nil {
		return nil, e
	}
	var out []doc
	for _, r := range rows[1:] {
		if len(r) >= 2 {
			out = append(out, doc{r[0], r[1]})
		}
	}
	return out, nil
}
func readJSONL(path string) ([]doc, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 64*1024), 1024*1024)
	var out []doc
	for s.Scan() {
		var d doc
		if e = json.Unmarshal(s.Bytes(), &d); e != nil {
			return nil, e
		}
		out = append(out, d)
	}
	return out, s.Err()
}
func main() {
	ref := flag.String("reference", "", "training CSV")
	candidate := flag.String("candidate", "", "candidate JSONL")
	flag.Parse()
	r, e := readCSV(*ref)
	if e != nil {
		panic(e)
	}
	c, e := readJSONL(*candidate)
	if e != nil {
		panic(e)
	}
	type match struct {
		Candidate, Reference string
		Distance             int
		Shingle              bool
	}
	type indexedDoc struct {
		doc        doc
		simhash    uint64
		shingles   overlap.Print
		normalized string
	}
	indexedReference := make([]indexedDoc, len(r))
	for i, y := range r {
		indexedReference[i] = indexedDoc{doc: y, simhash: dedup.Fingerprint(y.Text), shingles: overlap.Fingerprint(y.Text), normalized: dedup.Normalize(y.Text)}
	}
	var exact, sim10, shingle []match
	for _, x := range c {
		xf := dedup.Fingerprint(x.Text)
		xp := overlap.Fingerprint(x.Text)
		xn := dedup.Normalize(x.Text)
		for _, iy := range indexedReference {
			y := iy.doc
			d := bits.OnesCount64(xf ^ iy.simhash)
			same := xn == iy.normalized
			if same {
				exact = append(exact, match{x.ID, y.ID, d, false})
			}
			if d <= 10 {
				sim10 = append(sim10, match{x.ID, y.ID, d, false})
			}
			if overlap.Similar(xp, iy.shingles) {
				shingle = append(shingle, match{x.ID, y.ID, d, true})
			}
		}
	}
	out := map[string]any{"reference_documents": len(r), "candidate_documents": len(c), "normalized_exact": len(exact), "simhash_distance_le_10": len(sim10), "shingle_jaccard_matches": len(shingle), "exact_matches": exact, "simhash_matches": sim10, "shingle_matches": shingle}
	e = json.NewEncoder(os.Stdout).Encode(out)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
