// Package entityeval scores post-validation extractions against independently
// annotated entities. Matching is per document, never across the corpus.
package entityeval

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/entities"
)

type record struct {
	ID       string             `json:"id"`
	Entities *entities.Entities `json:"entities"`
}

type Counts struct {
	TP int `json:"tp"`
	FP int `json:"fp"`
	FN int `json:"fn"`
}

type Score struct {
	Counts
	Precision *float64 `json:"precision"`
	Recall    *float64 `json:"recall"`
	F1        *float64 `json:"f1"`
}

type Report struct {
	Documents  int              `json:"documents"`
	Micro      Score            `json:"micro"`
	ByCategory map[string]Score `json:"by_category"`
}

func ratio(numerator, denominator int) *float64 {
	if denominator == 0 {
		return nil
	}
	v := float64(numerator) / float64(denominator)
	return &v
}

func (c Counts) score() Score {
	return Score{c, ratio(c.TP, c.TP+c.FP), ratio(c.TP, c.TP+c.FN), ratio(2*c.TP, 2*c.TP+c.FP+c.FN)}
}

func byCategory(e entities.Entities) map[string][]string {
	return map[string][]string{
		"ips": e.IOCs.IPs, "hashes": e.IOCs.Hashes, "domains": e.IOCs.Domains, "urls": e.IOCs.URLs,
		"cves": e.CVEs, "malware": e.Malware, "threat_actors": e.ThreatActors, "infrastructure": e.Infrastructure,
	}
}

func set(category string, values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		switch category {
		case "hashes", "domains", "cves":
			v = strings.ToLower(v)
		}
		out[v] = struct{}{}
	}
	return out
}

func records(r io.Reader) (map[string]entities.Entities, error) {
	out := make(map[string]entities.Entities)
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for line := 1; scanner.Scan(); line++ {
		decoder := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
		decoder.DisallowUnknownFields()
		var row record
		if err := decoder.Decode(&row); err != nil || strings.TrimSpace(row.ID) == "" ||
			row.ID != strings.TrimSpace(row.ID) || row.Entities == nil ||
			decoder.Decode(new(any)) != io.EOF {
			return nil, fmt.Errorf("line %d: expected one object with nonempty id and an entities object", line)
		}
		if _, exists := out[row.ID]; exists {
			return nil, fmt.Errorf("line %d: duplicate id", line)
		}
		for _, values := range byCategory(*row.Entities) {
			for _, value := range values {
				if strings.TrimSpace(value) == "" {
					return nil, fmt.Errorf("line %d: empty entity value", line)
				}
			}
		}
		out[row.ID] = *row.Entities
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, errors.New("empty dataset")
	}
	return out, nil
}

func Evaluate(gold, predictions io.Reader) (Report, error) {
	want, err := records(gold)
	if err != nil {
		return Report{}, fmt.Errorf("gold: %w", err)
	}
	got, err := records(predictions)
	if err != nil {
		return Report{}, fmt.Errorf("predictions: %w", err)
	}
	if len(want) != len(got) {
		return Report{}, errors.New("gold and predictions must cover the same document IDs")
	}

	categories := make(map[string]Counts)
	for category := range byCategory(entities.Entities{}) {
		categories[category] = Counts{}
	}
	for id, expected := range want {
		actual, exists := got[id]
		if !exists {
			return Report{}, errors.New("gold and predictions must cover the same document IDs")
		}
		actualGroups := byCategory(actual)
		for category, values := range byCategory(expected) {
			expectedSet, actualSet := set(category, values), set(category, actualGroups[category])
			n := categories[category]
			n.FN += len(expectedSet)
			for value := range actualSet {
				if _, ok := expectedSet[value]; ok {
					n.TP++
					n.FN--
				} else {
					n.FP++
				}
			}
			categories[category] = n
		}
	}
	var total Counts
	report := Report{Documents: len(want), ByCategory: make(map[string]Score)}
	for category, n := range categories {
		total.TP += n.TP
		total.FP += n.FP
		total.FN += n.FN
		report.ByCategory[category] = n.score()
	}
	report.Micro = total.score()
	return report, nil
}
