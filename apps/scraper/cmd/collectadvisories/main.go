// collectadvisories collects a bounded, unscored reserve of public advisories.
// It never authenticates, follows redirects, bypasses limits or writes production.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/S-rvo/BlueWhiteThreat/pkg/csamfilter"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type advisory struct {
	ID              string `json:"ghsa_id"`
	CVE             string `json:"cve_id"`
	URL             string `json:"html_url"`
	Summary         string `json:"summary"`
	Description     string `json:"description"`
	PublishedAt     string `json:"published_at"`
	Vulnerabilities []struct {
		Package struct {
			Ecosystem string `json:"ecosystem"`
			Name      string `json:"name"`
		} `json:"package"`
	} `json:"vulnerabilities"`
}

func nextPage(link string) (string, error) {
	for _, part := range strings.Split(link, ",") {
		if strings.Contains(part, `rel="next"`) {
			start := strings.Index(part, "<")
			end := strings.Index(part, ">")
			if start < 0 || end <= start {
				return "", fmt.Errorf("invalid pagination link")
			}
			raw := part[start+1 : end]
			u, err := url.Parse(raw)
			if err != nil || u.Scheme != "https" || u.Host != "api.github.com" || u.Path != "/advisories" {
				return "", fmt.Errorf("unexpected pagination URL")
			}
			return raw, nil
		}
	}
	return "", nil
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	out := flag.String("out", "../classifier/data/automatic-v2-2026-10-19/advisory-reserve.jsonl", "new private unscored output")
	pages := flag.Int("pages", 4, "maximum requests (1..12)")
	skipPages := flag.Int("skip-pages", 0, "cursor pages to traverse before collecting (0..48)")
	flag.Parse()
	if *pages < 1 || *pages > 12 {
		return fmt.Errorf("invalid bounded page count")
	}
	if *skipPages < 0 || *skipPages > 48 || *skipPages+*pages > 60 {
		return fmt.Errorf("invalid cursor skip")
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
	count, excluded, requests := 0, 0, 0
	seen := map[string]bool{}
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	raw := "https://api.github.com/advisories?type=reviewed&per_page=100&sort=published&direction=desc"
	for page := 0; page < *skipPages+*pages && raw != ""; page++ {
		if page > 0 {
			time.Sleep(2 * time.Second)
		}
		req, err := http.NewRequest("GET", raw, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", "BlueWhiteThreat-Evaluation/0.2 (bounded public research)")
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
		response, err := client.Do(req)
		if err != nil {
			return err
		}
		requests++
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 16*1024*1024+1))
		response.Body.Close()
		if response.StatusCode != 200 {
			return fmt.Errorf("HTTP %d; collection stopped without retry", response.StatusCode)
		}
		if readErr != nil {
			return readErr
		}
		if len(body) > 16*1024*1024 {
			return fmt.Errorf("response limit exceeded")
		}
		var records []advisory
		if err = json.Unmarshal(body, &records); err != nil {
			return err
		}
		if page >= *skipPages {
			for _, a := range records {
				text := strings.TrimSpace(html.UnescapeString(a.Summary + "\n" + a.Description))
				packages := []string{}
				for _, vulnerability := range a.Vulnerabilities {
					name := strings.ToLower(strings.TrimSpace(vulnerability.Package.Name))
					ecosystem := strings.ToLower(strings.TrimSpace(vulnerability.Package.Ecosystem))
					if name != "" {
						packages = append(packages, ecosystem+":"+name)
					}
				}
				sort.Strings(packages)
				group := "ghsa:" + a.ID
				if len(packages) > 0 {
					group = "package:" + packages[0]
				} else if a.CVE != "" {
					group = "cve:" + a.CVE
				}
				u, err := url.Parse(a.URL)
				if err != nil || u.Scheme != "https" || u.Host != "github.com" || !strings.HasPrefix(u.Path, "/advisories/GHSA-") {
					return fmt.Errorf("unexpected advisory URL")
				}
				if a.ID == "" || len([]rune(text)) < 100 || len(text) > 100000 || csamfilter.IsBlockedURL(a.URL) || csamfilter.ContainsBlockedContent(text) || seen[group] {
					excluded++
					continue
				}
				seen[group] = true
				d := map[string]string{"id": "ghsa:" + a.ID, "group": group, "text": text, "url": a.URL, "source": "github-advisories", "source_type": "advisory", "stratum": "en_advisory", "language": "en", "source_date": a.PublishedAt, "collected_at": time.Now().UTC().Format(time.RFC3339)}
				if err = enc.Encode(d); err != nil {
					return err
				}
				count++
			}
		}
		raw, err = nextPage(response.Header.Get("Link"))
		if err != nil {
			return err
		}
	}
	if err = f.Sync(); err != nil {
		return err
	}
	summary := map[string]any{"retained": count, "excluded": excluded, "requests": requests, "skipped_cursor_pages": *skipPages, "collected_pages": *pages, "unscored": true, "purpose": "reserve; not part of the frozen 400-document campaign", "source": "public GitHub-reviewed advisories; no automatic relevance label"}
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
