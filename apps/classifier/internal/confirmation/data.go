// Package confirmation freezes a candidate and guards its final evaluation.
package confirmation

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"math/bits"
	"os"
	"sort"
	"strings"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/dedup"
)

type Document struct {
	ID     string  `json:"id"`
	Text   string  `json:"text"`
	Source string  `json:"source,omitempty"`
	Label  float64 `json:"label"`
}

// ReadDocuments rejects incomplete annotations instead of silently dropping
// them and changing the denominator of the evaluation. Pools use id,text.
func ReadDocuments(path string, labeled bool) ([]Document, error) {
	return readDocuments(path, labeled, false)
}

// Historical whitespace-only documents must retain their IDs in the reference
// archive even though they are forbidden in any new annotation or sampling pool.
func readDocuments(path string, labeled, allowBlankReference bool) ([]Document, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("CSV header: %w", err)
	}
	expected := "id,text"
	if labeled {
		expected += ",label"
	}
	sourceHeader := "id,text,source"
	if labeled {
		sourceHeader += ",label"
	}
	actual := strings.Join(header, ",")
	if actual != expected && actual != sourceHeader {
		return nil, fmt.Errorf("expected header %s or %s", expected, sourceHeader)
	}
	hasSource := actual == sourceHeader
	seen := map[string]bool{}
	var docs []Document
	for line := 2; ; line++ {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", line, err)
		}
		id := strings.TrimSpace(row[0])
		if id == "" || (normalize(row[1]) == "" && !allowBlankReference) || seen[id] {
			return nil, fmt.Errorf("row %d: empty id/text or repeated id", line)
		}
		d := Document{ID: id, Text: row[1]}
		if hasSource {
			d.Source = strings.TrimSpace(row[2])
		}
		if labeled {
			labelIndex := len(row) - 1
			switch row[labelIndex] {
			case "0":
				d.Label = 0
			case "1":
				d.Label = 1
			default:
				return nil, fmt.Errorf("row %d: label must be exactly 0 or 1", line)
			}
		}
		seen[id] = true
		docs = append(docs, d)
	}
	if len(docs) == 0 {
		return nil, fmt.Errorf("empty dataset")
	}
	return docs, nil
}

func normalize(s string) string { return strings.Join(strings.Fields(strings.ToLower(s)), " ") }
func digest(b []byte) string    { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }

type fingerprint struct {
	ID, Exact string
	Source    string
	Near      uint64
}

func printOf(d Document) fingerprint {
	return fingerprint{ID: d.ID, Exact: digest([]byte(normalize(d.Text))), Source: d.Source, Near: dedup.Fingerprint(d.Text)}
}
func prints(docs []Document) []fingerprint {
	out := make([]fingerprint, len(docs))
	for i, d := range docs {
		out[i] = printOf(d)
	}
	return out
}

// Compare against EVERY document, not just a cluster representative. SimHash
// is a conservative lexical guard, not proof of semantic independence.
func collision(p fingerprint, prior []fingerprint) string {
	for _, q := range prior {
		if p.ID == q.ID {
			return "previously used id"
		}
		if p.Exact == q.Exact {
			return "normalized duplicate"
		}
		if bits.OnesCount64(p.Near^q.Near) <= 10 {
			return "near duplicate (SimHash distance <= 10)"
		}
	}
	return ""
}
func checkBatch(docs, corpus []Document) error {
	prior := prints(corpus)
	for _, d := range docs {
		p := printOf(d)
		if reason := collision(p, prior); reason != "" {
			return fmt.Errorf("document %s: %s in reference corpus or confirmation batch", d.ID, reason)
		}
		prior = append(prior, p)
	}
	return nil
}

// batchDigest ignores row order, but records the exact annotated text/label.
func batchDigest(docs []Document) string {
	rows := append([]Document(nil), docs...)
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	var b strings.Builder
	w := csv.NewWriter(&b)
	for _, d := range rows {
		_ = w.Write([]string{d.ID, d.Text, d.Source, fmt.Sprint(d.Label)})
	}
	w.Flush()
	return digest([]byte(b.String()))
}
