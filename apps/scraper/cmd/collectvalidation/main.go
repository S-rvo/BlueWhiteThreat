// collectvalidation collects a bounded, unscored validation pool from public
// discussion feeds, advisories and the Stack Exchange API. No production writes.
package main

import (
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/sites"
	"github.com/S-rvo/BlueWhiteThreat/pkg/csamfilter"
	"golang.org/x/net/html/charset"
)

type document struct {
	ID         string `json:"id"`
	Text       string `json:"text"`
	URL        string `json:"url"`
	Source     string `json:"source"`
	Language   string `json:"language"`
	Kind       string `json:"source_type"`
	Group      string `json:"group"`
	Stratum    string `json:"stratum"`
	Collected  string `json:"collected_at"`
	SourceDate string `json:"source_date,omitempty"`
}
type item struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Body        string `xml:"encoded"`
	Description string `xml:"description"`
}
type source struct{ URL, Name, Language, Kind, Stratum string }

var threadID = regexp.MustCompile(`(?:[?&]t=|/d)([0-9]+)`)
var sources = []source{
	{"https://www.developpez.net/forums/external.php?type=RSS2&forumids=1559", "developpez", "fr", "forum_post", "fr_forum"},
	{"https://www.developpez.net/forums/external.php?type=RSS2&forumids=180", "developpez", "fr", "forum_post", "fr_forum"},
	{"https://www.developpez.net/forums/external.php?type=RSS2&forumids=331", "developpez", "fr", "forum_post", "fr_forum"},
	{"https://www.developpez.net/forums/external.php?type=RSS2&forumids=817", "developpez", "fr", "forum_post", "fr_forum"},
	{"https://cert.cyber.gouv.fr/avis/feed/", "cert-fr", "fr", "advisory", "fr_advisory"},
	{"https://cert.cyber.gouv.fr/alerte/feed/", "cert-fr", "fr", "advisory", "fr_advisory"},
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func makeDocument(s source, title, body, link, now string) (*document, error) {
	if csamfilter.IsBlockedURL(link) || csamfilter.ContainsBlockedContent(title+"\n"+body) {
		return nil, nil
	}
	if s.Name == "cert-fr" {
		link = strings.Replace(link, "https://www.cert.ssi.gouv.fr/", "https://cert.cyber.gouv.fr/", 1)
	}
	parsed, err := url.Parse(link)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme == "http" && strings.HasSuffix(parsed.Hostname(), "developpez.net") {
		parsed.Scheme = "https"
	}
	allowed := map[string]string{"developpez": "www.developpez.net", "cert-fr": "cert.cyber.gouv.fr", "security": "security.stackexchange.com", "superuser": "superuser.com", "askubuntu": "askubuntu.com"}
	if parsed.Scheme != "https" || parsed.Hostname() != allowed[s.Name] {
		return nil, fmt.Errorf("unexpected item host for %s", s.Name)
	}
	link = parsed.String()
	group := link
	if s.Name == "developpez" {
		m := threadID.FindStringSubmatch(link)
		if len(m) < 2 {
			return nil, fmt.Errorf("no thread id in forum URL")
		}
		group = "developpez:" + m[1]
		link = "https://www.developpez.net/forums/showthread.php?t=" + m[1]
	}
	text, err := sites.ExtractMainContent("<article><h1>" + html.EscapeString(html.UnescapeString(title)) + "</h1>" + body + "</article>")
	if err != nil {
		return nil, err
	}
	if len([]rune(text)) < 100 || len(text) > 100000 || csamfilter.ContainsBlockedContent(text) {
		return nil, nil
	}
	return &document{ID: group, Group: group, Text: text, URL: link, Source: s.Name, Language: s.Language, Kind: s.Kind, Stratum: s.Stratum, Collected: now}, nil
}
func stackQuestionsURL(site string, page int, maxDate int64) string {
	query := "https://api.stackexchange.com/2.3/questions?site=" + url.QueryEscape(site) + "&pagesize=100&page=" + strconv.Itoa(page) + "&order=desc&sort=creation&filter=withbody"
	if maxDate > 0 {
		query += "&max=" + strconv.FormatInt(maxDate, 10)
	}
	return query
}
func formatSourceDate(unixSeconds int64) string {
	if unixSeconds <= 0 {
		return ""
	}
	return time.Unix(unixSeconds, 0).UTC().Format(time.RFC3339)
}
func run() error {
	output := flag.String("out", "../classifier/data/automatic-v2-2026-10-19/new-pool.jsonl", "new private JSONL output")
	only := flag.String("only-stratum", "", "optional fixed source stratum")
	pageFrom := flag.Int("page-from", 3, "first Stack Exchange page (1..25)")
	pageTo := flag.Int("page-to", 0, "last Stack Exchange page (1..25); 0 uses source default")
	securityMaxDate := flag.Int64("security-max-date", 0, "optional Stack Exchange creation-date upper bound (Unix seconds) for an en_security time window")
	flag.Parse()
	if *pageFrom < 1 || *pageFrom > 25 || *pageTo < 0 || *pageTo > 25 || (*pageTo != 0 && *pageTo < *pageFrom) || *securityMaxDate < 0 || (*securityMaxDate > 0 && *only != "en_security") {
		return fmt.Errorf("invalid bounded page range")
	}
	if err := os.MkdirAll(filepath.Dir(*output), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	now := time.Now().UTC().Format(time.RFC3339)
	client := &http.Client{Timeout: 25 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	counts := map[string]int{}
	seen := map[string]bool{}
	excluded := 0
	requests := 0
	failures := []string{}
	get := func(raw string) ([]byte, error) {
		if requests > 0 {
			time.Sleep(1500 * time.Millisecond)
		}
		requests++
		if csamfilter.IsBlockedURL(raw) {
			return nil, fmt.Errorf("blocked URL")
		}
		req, err := http.NewRequest("GET", raw, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "BlueWhiteThreat-Evaluation/0.2 (bounded public research)")
		r, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer r.Body.Close()
		if r.StatusCode != 200 {
			return nil, fmt.Errorf("HTTP %d; no bypass/retry", r.StatusCode)
		}
		b, err := io.ReadAll(io.LimitReader(r.Body, 16*1024*1024+1))
		if len(b) > 16*1024*1024 {
			return nil, fmt.Errorf("response too large")
		}
		return b, err
	}
	save := func(d *document) error {
		if d == nil {
			excluded++
			return nil
		}
		if seen[d.Group] {
			return nil
		}
		seen[d.Group] = true
		if err := enc.Encode(d); err != nil {
			return err
		}
		counts[d.Stratum]++
		return nil
	}
	// Source list and page range are fixed before any annotation/model score.
	for _, s := range sources {
		if *only != "" && s.Stratum != *only {
			continue
		}
		b, err := get(s.URL)
		if err != nil {
			failures = append(failures, s.Name+": "+err.Error())
			continue
		}
		var rss struct {
			Items []item `xml:"channel>item"`
		}
		dec := xml.NewDecoder(strings.NewReader(string(b)))
		dec.CharsetReader = charset.NewReaderLabel
		if err = dec.Decode(&rss); err != nil {
			return err
		}
		for _, i := range rss.Items {
			if s.Name == "cert-fr" {
				canonical := strings.Replace(i.Link, "https://www.cert.ssi.gouv.fr/", "https://cert.cyber.gouv.fr/", 1)
				full, fetchErr := get(canonical)
				if fetchErr != nil {
					failures = append(failures, "cert-fr item: "+fetchErr.Error())
					continue
				}
				i.Body = string(full)
			}
			body := i.Body
			if body == "" {
				body = i.Description
			}
			d, err := makeDocument(s, i.Title, body, i.Link, now)
			if err != nil {
				return err
			}
			if err = save(d); err != nil {
				return err
			}
		}
	}
	for _, site := range []string{"security", "superuser", "askubuntu"} {
		stratum := "en_general"
		if site == "security" {
			stratum = "en_security"
		}
		if *only != "" && *only != stratum {
			continue
		}
		end := 5
		if site == "security" {
			end = 9
		}
		if *pageTo != 0 {
			end = *pageTo
		}
		for page := *pageFrom; page <= end; page++ {
			maxDate := int64(0)
			if site == "security" {
				maxDate = *securityMaxDate
			}
			raw := stackQuestionsURL(site, page, maxDate)
			b, err := get(raw)
			if err != nil {
				failures = append(failures, site+": "+err.Error())
				break
			}
			var p struct {
				Items []struct {
					ID      int    `json:"question_id"`
					Created int64  `json:"creation_date"`
					Title   string `json:"title"`
					Body    string `json:"body"`
					Link    string `json:"link"`
				} `json:"items"`
				Backoff int  `json:"backoff"`
				Quota   int  `json:"quota_remaining"`
				Error   int  `json:"error_id"`
				HasMore bool `json:"has_more"`
			}
			if err = json.Unmarshal(b, &p); err != nil {
				return err
			}
			if p.Error != 0 {
				return fmt.Errorf("API error %d", p.Error)
			}
			for _, q := range p.Items {
				d, err := makeDocument(source{Name: site, Language: "en", Kind: "question", Stratum: stratum}, q.Title, q.Body, q.Link, now)
				if err != nil {
					return err
				}
				if d != nil {
					d.ID = fmt.Sprintf("stackexchange:%s:%d", site, q.ID)
					d.Group = d.ID
					d.SourceDate = formatSourceDate(q.Created)
				}
				if err = save(d); err != nil {
					return err
				}
			}
			if p.Backoff > 0 || p.Quota < 10 {
				return fmt.Errorf("API backoff/low quota; stopped with partial pool")
			}
			if !p.HasMore {
				break
			}
		}
	}
	if err = f.Sync(); err != nil {
		return err
	}
	summary := map[string]any{"counts": counts, "safety_or_size_exclusions": excluded, "requests": requests, "failures": failures, "collected_at": now, "limitations": []string{"Public security discussions and advisories, not criminal marketplaces or Tor.", "RSS body is the publisher supplied content; short summaries are not full-page evidence.", "Different source prevalence from production; report metrics by stratum.", "No answers, attachments, accounts or credentials collected."}}
	b, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	report, err := os.OpenFile(*output+".summary.json", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer report.Close()
	if _, err = report.Write(append(b, '\n')); err != nil {
		return err
	}
	log.Printf("Collected unscored pool: %v, exclusions=%d requests=%d failures=%d", counts, excluded, requests, len(failures))
	return nil
}
