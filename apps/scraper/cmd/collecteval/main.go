// collecteval builds an unscored public Q&A pilot pool. It does not enqueue
// production URLs or modify MongoDB. Run from apps/scraper.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/sites"
	"github.com/S-rvo/BlueWhiteThreat/pkg/csamfilter"
)

type question struct {
	ID      int    `json:"question_id"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	Link    string `json:"link"`
	Created int64  `json:"creation_date"`
}
type document struct {
	ID         string `json:"id"`
	Text       string `json:"text"`
	URL        string `json:"url"`
	Source     string `json:"source"`
	Language   string `json:"language"`
	SourceType string `json:"source_type"`
	Group      string `json:"group"`
	Created    string `json:"source_created_at"`
	Collected  string `json:"collected_at"`
}

var sources = []struct{ site, language string }{{"security", "en"}, {"superuser", "en"}, {"askubuntu", "en"}, {"french", "fr"}}

func eligible(q question, site, language, now string) (*document, error) {
	if csamfilter.IsBlockedURL(q.Link) || csamfilter.ContainsBlockedContent(q.Title+"\n"+q.Body) {
		return nil, nil
	}
	parsed, err := url.Parse(q.Link)
	expected := map[string]string{"security": "security.stackexchange.com", "superuser": "superuser.com", "askubuntu": "askubuntu.com", "french": "french.stackexchange.com"}
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() != expected[site] || q.ID <= 0 {
		return nil, fmt.Errorf("invalid canonical question URL or id")
	}
	// Escape the title, preserving inline markup in the body exactly as the
	// production main-content extractor expects it.
	text, err := sites.ExtractMainContent("<article><h1>" + html.EscapeString(html.UnescapeString(q.Title)) + "</h1>" + q.Body + "</article>")
	if err != nil {
		return nil, err
	}
	if csamfilter.ContainsBlockedContent(text) || len([]rune(text)) < 100 || len(text) > 200000 {
		return nil, nil
	}
	id := fmt.Sprintf("stackexchange:%s:%d", site, q.ID)
	return &document{ID: id, Text: text, URL: q.Link, Source: site, Language: language, SourceType: "public_qa_pilot", Group: id, Created: time.Unix(q.Created, 0).UTC().Format(time.RFC3339), Collected: now}, nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	out := flag.String("out", "../classifier/data/automatic-eval-2026-10-19/pool.jsonl", "new private JSONL file")
	pages := flag.Int("pages-per-source", 2, "bounded pages of 100 questions per source (1-3)")
	flag.Parse()
	if *pages < 1 || *pages > 3 {
		return fmt.Errorf("pages-per-source must be 1..3")
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	client := &http.Client{Timeout: 25 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	now := time.Now().UTC().Format(time.RFC3339)
	counts := map[string]int{}
	rejected := 0
	requests := 0
	seen := map[string]bool{}
	for _, source := range sources {
		for page := 1; page <= *pages; page++ {
			if requests > 0 {
				time.Sleep(1500 * time.Millisecond)
			}
			endpoint := "https://api.stackexchange.com/2.3/questions?site=" + source.site + "&pagesize=100&page=" + strconv.Itoa(page) + "&order=desc&sort=creation&filter=withbody"
			req, err := http.NewRequest(http.MethodGet, endpoint, nil)
			if err != nil {
				return err
			}
			req.Header.Set("User-Agent", "BlueWhiteThreat-evaluation/0.1 (bounded public Q&A pilot)")
			resp, err := client.Do(req)
			requests++
			if err != nil {
				return fmt.Errorf("%s request: %w", source.site, err)
			}
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, 16*1024*1024+1))
			readErr = errors.Join(readErr, resp.Body.Close())
			if readErr != nil {
				return readErr
			}
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("%s HTTP %d; stopped without retry/bypass", source.site, resp.StatusCode)
			}
			if len(body) > 16*1024*1024 {
				return fmt.Errorf("response exceeds limit")
			}
			var payload struct {
				Items   []question `json:"items"`
				Backoff int        `json:"backoff"`
				ErrorID int        `json:"error_id"`
				HasMore bool       `json:"has_more"`
				Quota   int        `json:"quota_remaining"`
			}
			if err = json.Unmarshal(body, &payload); err != nil {
				return fmt.Errorf("invalid API response: %w", err)
			}
			if payload.ErrorID != 0 {
				return fmt.Errorf("API error %d", payload.ErrorID)
			}
			for _, q := range payload.Items {
				d, err := eligible(q, source.site, source.language, now)
				if err != nil {
					return err
				}
				if d == nil {
					rejected++
					continue
				}
				if seen[d.ID] {
					continue
				}
				seen[d.ID] = true
				if err = enc.Encode(d); err != nil {
					return err
				}
				counts[source.site]++
			}
			if payload.Backoff > 0 || payload.Quota < 10 {
				return fmt.Errorf("API requested backoff/low quota; stopped, partial pool retained")
			}
			if !payload.HasMore {
				break
			}
		}
	}
	if err = f.Sync(); err != nil {
		return err
	}
	summary := struct {
		Collected   string         `json:"collected_at"`
		Counts      map[string]int `json:"by_source"`
		Excluded    int            `json:"safety_or_size_exclusions"`
		Requests    int            `json:"requests"`
		Limitations []string       `json:"limitations"`
	}{now, counts, rejected, requests, []string{"Public Q&A pilot, not representative of Tor forums or access-sale posts.", "French source concerns language questions, not French cyber threats.", "Question bodies only; no answers/comments/images or model-based selection.", "Safety filter is a lexical project guard, not a semantic guarantee.", "API bodies passed through production HTML extraction; browser/network path is not tested here."}}
	b, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	report, err := os.OpenFile(strings.TrimSuffix(*out, ".jsonl")+".summary.json", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err = report.Write(append(b, '\n')); err != nil {
		return errors.Join(err, report.Close())
	}
	if err = report.Close(); err != nil {
		return err
	}
	log.Printf("Collected safe pilot questions by source: %v; excluded %d; requests %d", counts, rejected, requests)
	return nil
}
