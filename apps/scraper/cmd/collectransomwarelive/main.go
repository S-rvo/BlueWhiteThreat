// collectransomwarelive builds a bounded, unscored pool of ransomware claims.
// Source membership is provenance, never an automatic training label.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const maxResponse = 32 * 1024 * 1024
const maxRetained = 25000

type victim struct {
	Activity    string `json:"activity"`
	Country     string `json:"country"`
	Description string `json:"description"`
	Discovered  string `json:"discovered"`
	Group       string `json:"group"`
	GroupName   string `json:"group_name"`
	PostTitle   string `json:"post_title"`
	Victim      string `json:"victim"`
	URL         string `json:"url"`
}

type document struct {
	ID             string `json:"id"`
	Group          string `json:"group"`
	Text           string `json:"text"`
	URL            string `json:"url"`
	Source         string `json:"source"`
	SourceType     string `json:"source_type"`
	Style          string `json:"style"`
	CollectedAt    string `json:"collected_at"`
	SourceDate     string `json:"source_date"`
	SourceActor    string `json:"source_actor"`
	LabelAuthority string `json:"label_authority"`
}

func clean(s string) string { return strings.Join(strings.Fields(html.UnescapeString(s)), " ") }

func normalize(v victim) *document {
	name := clean(v.PostTitle)
	if name == "" {
		name = clean(v.Victim)
	}
	description := clean(v.Description)
	if strings.EqualFold(description, "N/A") {
		description = ""
	}
	// Keep the observable post text, not source-added country/activity/group
	// metadata. This limits trivial source shortcuts in later experiments.
	text := strings.TrimSpace(name + "\n" + description)
	if len([]rune(text)) < 100 || name == "" || v.Discovered == "" {
		return nil
	}
	group := clean(v.Group)
	if group == "" {
		group = clean(v.GroupName)
	}
	sum := sha256.Sum256([]byte(v.Discovered + "\x00" + group + "\x00" + name))
	id := fmt.Sprintf("ransomware-live:%x", sum[:12])
	validationGroup := id
	if group != "" {
		validationGroup = "ransomware-actor:" + strings.ToLower(group)
	}
	return &document{ID: id, Group: validationGroup, Text: text, URL: v.URL, Source: "ransomware.live", SourceType: "ransomware_claim", Style: "target", SourceDate: v.Discovered, SourceActor: group, LabelAuthority: "unscored; source membership is not a label"}
}

func months(start, end time.Time) ([]time.Time, error) {
	if end.Before(start) {
		return nil, fmt.Errorf("end precedes start")
	}
	var out []time.Time
	for at := start; !at.After(end); at = at.AddDate(0, 1, 0) {
		out = append(out, at)
		if len(out) > 24 {
			return nil, fmt.Errorf("date range exceeds 24 months")
		}
	}
	return out, nil
}

func run() error {
	out := flag.String("out", "../classifier/data/ransomware-live/pool.jsonl", "new private unscored JSONL")
	startRaw := flag.String("start", "2026-01", "first month, YYYY-MM")
	endRaw := flag.String("end", time.Now().UTC().Format("2006-01"), "last month, YYYY-MM")
	limit := flag.Int("limit", maxRetained, "maximum retained documents (1..25000)")
	flag.Parse()
	if *limit < 1 || *limit > maxRetained {
		return fmt.Errorf("invalid limit")
	}
	start, err := time.Parse("2006-01", *startRaw)
	if err != nil {
		return fmt.Errorf("invalid start: %w", err)
	}
	end, err := time.Parse("2006-01", *endRaw)
	if err != nil {
		return fmt.Errorf("invalid end: %w", err)
	}
	period, err := months(start, end)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(*out), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	now := time.Now().UTC().Format(time.RFC3339)
	var docs []document
	rejected, requests := 0, 0
	seen := map[string]bool{}
	for _, month := range period {
		if requests > 0 {
			time.Sleep(2 * time.Second)
		}
		endpoint := "https://api.ransomware.live/v2/victims/" + month.Format("2006/01")
		req, _ := http.NewRequest(http.MethodGet, endpoint, nil)
		req.Header.Set("User-Agent", "BlueWhiteThreat-SourceAudit/0.1")
		resp, requestErr := client.Do(req)
		requests++
		if requestErr != nil {
			return requestErr
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
		readErr = errors.Join(readErr, resp.Body.Close())
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("GET %s: HTTP %d", endpoint, resp.StatusCode)
		}
		if readErr != nil || len(body) > maxResponse {
			return fmt.Errorf("invalid response for %s", endpoint)
		}
		var rows []victim
		if err = json.Unmarshal(body, &rows); err != nil {
			return err
		}
		for _, row := range rows {
			doc := normalize(row)
			if doc == nil || seen[doc.ID] {
				rejected++
				continue
			}
			doc.CollectedAt = now
			seen[doc.ID] = true
			docs = append(docs, *doc)
		}
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].ID < docs[j].ID })
	if len(docs) > *limit {
		rejected += len(docs) - *limit
		docs = docs[:*limit]
	}
	enc := json.NewEncoder(f)
	for _, doc := range docs {
		if err = enc.Encode(doc); err != nil {
			return err
		}
	}
	if err = f.Sync(); err != nil {
		return err
	}
	summary := map[string]any{"retained": len(docs), "excluded_or_duplicate": rejected, "requests": requests, "months": len(period), "unscored": true, "automatic_labels": false, "source": "ransomware.live v2 public API"}
	sf, err := os.OpenFile(*out+".summary.json", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer sf.Close()
	if err = json.NewEncoder(sf).Encode(summary); err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(summary)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
