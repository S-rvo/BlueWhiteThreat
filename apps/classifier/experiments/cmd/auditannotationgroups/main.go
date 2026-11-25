// auditannotationgroups checks assistant proposals against train-only duplicate groups.
// It neither changes labels nor trains or evaluates a model.
package main

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/dedup"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/overlap"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/review"
)

type proposal struct {
	ID       string `json:"id"`
	Hash     string `json:"text_sha256"`
	Original int    `json:"original_human_label"`
	Label    *int   `json:"proposed_label"`
}

type member struct {
	ID       string `json:"id"`
	Original int    `json:"original_label"`
	Proposed *int   `json:"assistant_label"`
}

type cluster struct {
	Members          []member `json:"members"`
	OriginalConflict bool     `json:"original_conflict"`
	OverlayConflict  bool     `json:"overlay_conflict"`
	Changed          int      `json:"changed_documents"`
}

func group(ids, texts []string, labels map[string]int, proposals map[string]proposal) []cluster {
	prints := make([]overlap.Print, len(ids))
	keys := make([]string, len(ids))
	parents := make([]int, len(ids))
	for i := range ids {
		prints[i] = overlap.Fingerprint(texts[i])
		keys[i] = dedup.Normalize(texts[i])
		parents[i] = i
	}
	var root func(int) int
	root = func(i int) int {
		if parents[i] != i {
			parents[i] = root(parents[i])
		}
		return parents[i]
	}
	for i := range ids {
		for j := 0; j < i; j++ {
			if keys[i] == keys[j] || overlap.Similar(prints[i], prints[j]) {
				parents[root(i)] = root(j)
			}
		}
	}
	byRoot := map[int][]int{}
	for i := range ids {
		byRoot[root(i)] = append(byRoot[root(i)], i)
	}
	result := []cluster{}
	for _, indices := range byRoot {
		c := cluster{Members: []member{}}
		old, next := map[int]bool{}, map[int]bool{}
		affected := false
		for _, i := range indices {
			id := ids[i]
			label := labels[id]
			p, exists := proposals[id]
			affected = affected || exists
			old[label] = true
			if p.Label != nil {
				if *p.Label != label {
					c.Changed++
				}
				label = *p.Label
			}
			next[label] = true
			c.Members = append(c.Members, member{id, labels[id], p.Label})
		}
		if !affected {
			continue
		}
		c.OriginalConflict = len(old) > 1
		c.OverlayConflict = len(next) > 1
		sort.Slice(c.Members, func(i, j int) bool { return c.Members[i].ID < c.Members[j].ID })
		result = append(result, c)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Members[0].ID < result[j].Members[0].ID })
	return result
}

func readJSON(path string, target any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, target)
}

func run() error {
	annotations := flag.String("annotations", "data/persistent-annotations-assistant-20261015/assistant-decisions.json", "assistant decisions")
	out := flag.String("out", "data/persistent-annotations-assistant-20261015/group-audit.json", "new report path")
	flag.Parse()
	var artifact struct {
		Inputs    map[string]string `json:"inputs_sha256"`
		Documents []proposal        `json:"documents"`
	}
	if err := readJSON(*annotations, &artifact); err != nil {
		return err
	}
	for path, hash := range artifact.Inputs {
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if fmt.Sprintf("%x", sha256.Sum256(b)) != hash {
			return fmt.Errorf("input changed: %s", path)
		}
	}
	var protocol struct {
		IDs []string `json:"train_ids"`
	}
	if err := readJSON("data/score-investigation-20261013/protocol.json", &protocol); err != nil {
		return err
	}
	allowed := map[string]bool{}
	for _, id := range protocol.IDs {
		if allowed[id] {
			return fmt.Errorf("duplicate protocol ID")
		}
		allowed[id] = true
	}
	reviewedLabels, err := review.LoadReviewed("data/reviewed_labels.csv")
	if err != nil {
		return err
	}
	labels := map[string]int{}
	for id, l := range reviewedLabels {
		labels[id] = l.Label
	}
	f, err := os.Open("data/confirmation-campaign/reference.csv")
	if err != nil {
		return err
	}
	defer f.Close()
	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		return err
	}
	idCol, textCol := -1, -1
	for i, name := range header {
		if name == "id" {
			idCol = i
		}
		if name == "text" {
			textCol = i
		}
	}
	if idCol < 0 || textCol < 0 {
		return fmt.Errorf("missing corpus columns")
	}
	docs := map[string]string{}
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		id := record[idCol]
		if !allowed[id] {
			continue
		}
		if _, exists := docs[id]; exists {
			return fmt.Errorf("duplicate train text ID: %s", id)
		}
		docs[id] = record[textCol]
	}
	ps := map[string]proposal{}
	for _, p := range artifact.Documents {
		if !allowed[p.ID] {
			return fmt.Errorf("proposal outside train: %s", p.ID)
		}
		if _, exists := ps[p.ID]; exists {
			return fmt.Errorf("duplicate proposal")
		}
		if p.Label != nil && *p.Label != 0 && *p.Label != 1 {
			return fmt.Errorf("invalid proposal label")
		}
		if fmt.Sprintf("%x", sha256.Sum256([]byte(docs[p.ID]))) != p.Hash {
			return fmt.Errorf("proposal text changed")
		}
		if label, ok := labels[p.ID]; !ok || label != p.Original {
			return fmt.Errorf("original label mismatch")
		}
		ps[p.ID] = p
	}
	ids := append([]string(nil), protocol.IDs...)
	sort.Strings(ids)
	texts := make([]string, len(ids))
	for i, id := range ids {
		var ok bool
		texts[i], ok = docs[id]
		if !ok {
			return fmt.Errorf("missing train text")
		}
		if _, ok = labels[id]; !ok {
			return fmt.Errorf("missing human label")
		}
	}
	clusters := group(ids, texts, labels, ps)
	counts := map[string]int{"affected_groups": len(clusters), "new_conflicting_groups": 0, "changed_documents": 0, "related_unannotated_documents": 0}
	for _, c := range clusters {
		counts["changed_documents"] += c.Changed
		if c.OverlayConflict && !c.OriginalConflict {
			counts["new_conflicting_groups"]++
		}
		for _, m := range c.Members {
			if _, ok := ps[m.ID]; !ok {
				counts["related_unannotated_documents"]++
			}
		}
	}
	b, err := json.MarshalIndent(map[string]any{"scope": "train-only diagnostic; assistant labels not applied", "grouping": "connected normalized exact or internal/overlap.Similar; includes original conflicts for audit", "counts": counts, "groups": clusters}, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err = file.Write(append(b, '\n')); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(counts)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
