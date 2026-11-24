// collectreleases creates an unlabelled pool of official release notes. It is
// deliberately independent from the classifier and suitable for finding
// official-style negatives without assigning them a label in advance.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var repositories = []string{
	"kubernetes/kubernetes", "nodejs/node", "golang/go", "docker/compose",
	"prometheus/prometheus", "grafana/grafana", "hashicorp/terraform", "denoland/deno",
}

type release struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Tag     string `json:"tag_name"`
	Body    string `json:"body"`
	URL     string `json:"html_url"`
	Draft   bool   `json:"draft"`
	Created string `json:"published_at"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	out := flag.String("out", "../classifier/data/official-releases.jsonl", "exclusive output JSONL")
	perRepo := flag.Int("per-repo", 40, "releases requested per repository (1..100)")
	flag.Parse()
	if *perRepo < 1 || *perRepo > 100 {
		return fmt.Errorf("invalid per-repo count")
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(*out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	retained, excluded := 0, 0
	for i, repo := range repositories {
		if i > 0 {
			time.Sleep(time.Second)
		}
		raw := fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=%d", repo, *perRepo)
		req, err := http.NewRequest("GET", raw, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", "BlueWhiteThreat-DataAudit/0.3")
		req.Header.Set("Accept", "application/vnd.github+json")
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 16*1024*1024+1))
		resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("%s: HTTP %d", repo, resp.StatusCode)
		}
		if readErr != nil || len(body) > 16*1024*1024 {
			return fmt.Errorf("%s: invalid response", repo)
		}
		var records []release
		if err := json.Unmarshal(body, &records); err != nil {
			return err
		}
		for _, r := range records {
			u, err := url.Parse(r.URL)
			if err != nil || u.Scheme != "https" || u.Host != "github.com" || !strings.HasPrefix(u.Path, "/"+repo+"/releases/") {
				return fmt.Errorf("unexpected release URL")
			}
			text := strings.TrimSpace(html.UnescapeString(strings.TrimSpace(r.Name+" "+r.Tag) + "\n" + r.Body))
			if r.Draft || r.ID == 0 || len([]rune(text)) < 100 || len(text) > 100000 {
				excluded++
				continue
			}
			doc := map[string]any{"id": fmt.Sprintf("github-release:%d", r.ID), "group": fmt.Sprintf("github-release:%d", r.ID), "text": text, "url": r.URL, "source": "official-release", "publisher": repo, "source_type": "release_notes", "style": "official", "stratum": "en_official_release", "language": "en", "published_at": r.Created, "collected_at": time.Now().UTC().Format(time.RFC3339), "pool_role": "unlabelled_candidate"}
			if err := enc.Encode(doc); err != nil {
				return err
			}
			retained++
		}
	}
	if err := f.Sync(); err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"retained": retained, "excluded": excluded, "requests": len(repositories), "unscored": true})
}
