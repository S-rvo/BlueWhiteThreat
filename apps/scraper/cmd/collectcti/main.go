// collectcti builds an unscored, bounded pool from official public CTI feeds.
// Source membership is provenance, never an automatic relevance label.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
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

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/sites"
	"github.com/S-rvo/BlueWhiteThreat/pkg/csamfilter"
)

const (
	certEUFeed = "https://cert.europa.eu/publications/threat-intelligence-rss"
	certFRFeed = "https://www.cert.ssi.gouv.fr/cti/feed/"
	cisaFeed   = "https://www.cisa.gov/cybersecurity-advisories/cybersecurity-advisories.xml"
	sansFeed   = "https://isc.sans.edu/rssfeed_full.xml"
	kevFeed    = "https://www.cisa.gov/sites/default/files/feeds/known_exploited_vulnerabilities.json"
	maxBody    = 16 * 1024 * 1024
)

type document struct {
	ID             string `json:"id"`
	Group          string `json:"group"`
	Text           string `json:"text"`
	TextSHA256     string `json:"text_sha256"`
	URL            string `json:"url"`
	Source         string `json:"source"`
	Language       string `json:"language"`
	SourceType     string `json:"source_type"`
	Stratum        string `json:"stratum"`
	CollectedAt    string `json:"collected_at"`
	LabelAuthority string `json:"label_authority"`
}

type rss struct {
	Items []struct {
		Title string `xml:"title"`
		Link  string `xml:"link"`
	} `xml:"channel>item"`
}

type kevCatalog struct {
	Version string `json:"catalogVersion"`
	Items   []struct {
		CVE         string `json:"cveID"`
		Vendor      string `json:"vendorProject"`
		Product     string `json:"product"`
		Name        string `json:"vulnerabilityName"`
		Added       string `json:"dateAdded"`
		Description string `json:"shortDescription"`
		Action      string `json:"requiredAction"`
		Ransomware  string `json:"knownRansomwareCampaignUse"`
	} `json:"vulnerabilities"`
}

type collector struct {
	client   *http.Client
	requests int
}

func (c *collector) get(raw string) ([]byte, string, error) {
	if c.requests > 0 {
		time.Sleep(250 * time.Millisecond)
	}
	req, err := http.NewRequest(http.MethodGet, raw, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "BlueWhiteThreat-SourceAudit/0.1 (bounded public research)")
	response, err := c.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	c.requests++
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("GET %s: HTTP %d", raw, response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBody+1))
	if err != nil {
		return nil, "", err
	}
	if len(body) > maxBody {
		return nil, "", fmt.Errorf("GET %s: response too large", raw)
	}
	return body, response.Request.URL.String(), nil
}

func allowedArticle(raw, source string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return false
	}
	switch source {
	case "cert-eu-cti":
		return u.Hostname() == "cert.europa.eu" && strings.HasPrefix(u.Path, "/publications/threat-intelligence/")
	case "cert-fr-cti":
		return (u.Hostname() == "cert.cyber.gouv.fr" || u.Hostname() == "www.cert.cyber.gouv.fr" || u.Hostname() == "cert.ssi.gouv.fr" || u.Hostname() == "www.cert.ssi.gouv.fr") && strings.HasPrefix(u.Path, "/cti/")
	case "cisa-advisory":
		return u.Hostname() == "www.cisa.gov" && strings.HasPrefix(u.Path, "/news-events/cybersecurity-advisories/")
	case "sans-isc-diary":
		return u.Hostname() == "isc.sans.edu" && strings.HasPrefix(u.Path, "/diary/rss/")
	default:
		return false
	}
}

func normalizedDocument(source, language, title, rawURL string, body []byte, now string) (*document, error) {
	if !allowedArticle(rawURL, source) || csamfilter.IsBlockedURL(rawURL) {
		return nil, fmt.Errorf("unexpected %s article URL: %s", source, rawURL)
	}
	text, err := sites.ExtractMainContent(string(body))
	if err != nil {
		return nil, err
	}
	text = strings.TrimSpace(html.UnescapeString(title) + "\n" + text)
	if len([]rune(text)) < 200 || len(text) > 250000 || csamfilter.ContainsBlockedContent(text) {
		return nil, nil
	}
	sum := sha256.Sum256([]byte(text))
	idSum := sha256.Sum256([]byte(rawURL))
	id := source + ":" + hex.EncodeToString(idSum[:12])
	return &document{ID: id, Group: id, Text: text, TextSHA256: hex.EncodeToString(sum[:]), URL: rawURL, Source: source, Language: language, SourceType: "threat_intelligence_report", Stratum: language + "_cti", CollectedAt: now, LabelAuthority: "unscored; source membership is not a label"}, nil
}

func collectRSS(c *collector, feed, source, language, now string, limit int) ([]document, error) {
	body, _, err := c.get(feed)
	if err != nil {
		return nil, err
	}
	var parsed rss
	if err = xml.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	var out []document
	for _, item := range parsed.Items {
		if len(out) >= limit {
			break
		}
		link := strings.TrimSpace(item.Link)
		if !allowedArticle(link, source) {
			return nil, fmt.Errorf("unexpected URL in %s feed: %s", source, link)
		}
		page, finalURL, fetchErr := c.get(link)
		if fetchErr != nil {
			return nil, fetchErr
		}
		doc, normalizeErr := normalizedDocument(source, language, item.Title, finalURL, page, now)
		if normalizeErr != nil {
			return nil, normalizeErr
		}
		if doc != nil {
			out = append(out, *doc)
		}
	}
	return out, nil
}

func collectKEV(c *collector, now string, limit int, includeAll bool) ([]document, string, error) {
	body, finalURL, err := c.get(kevFeed)
	if err != nil {
		return nil, "", err
	}
	if finalURL != kevFeed {
		return nil, "", fmt.Errorf("unexpected KEV redirect: %s", finalURL)
	}
	var catalog kevCatalog
	if err = json.Unmarshal(body, &catalog); err != nil {
		return nil, "", err
	}
	var out []document
	for _, item := range catalog.Items {
		if len(out) >= limit {
			break
		}
		if (!includeAll && item.Ransomware != "Known") || item.CVE == "" {
			continue
		}
		text := strings.TrimSpace(strings.Join([]string{item.Name, item.Vendor + " " + item.Product, item.Description, item.Action}, "\n"))
		if len([]rune(text)) < 100 || csamfilter.ContainsBlockedContent(text) {
			continue
		}
		sum := sha256.Sum256([]byte(text))
		source, stratum := "cisa-kev-ransomware", "en_kev_ransomware"
		if includeAll {
			source, stratum = "cisa-kev", "en_kev"
		}
		out = append(out, document{ID: "cisa-kev:" + item.CVE, Group: "cve:" + item.CVE, Text: text, TextSHA256: hex.EncodeToString(sum[:]), URL: "https://www.cisa.gov/known-exploited-vulnerabilities-catalog?search_api_fulltext=" + url.QueryEscape(item.CVE), Source: source, Language: "en", SourceType: "exploited_vulnerability_metadata", Stratum: stratum, CollectedAt: now, LabelAuthority: "unscored auxiliary vocabulary; catalog membership is not a relevance label"})
	}
	return out, catalog.Version, nil
}

func run() error {
	outPath := flag.String("out", "../classifier/data/source-audit-2026-10-19/cti-pool.jsonl", "new private unscored JSONL")
	rssLimit := flag.Int("rss-limit", 10, "maximum documents per RSS source (1..25)")
	kevLimit := flag.Int("kev-limit", 100, "maximum ransomware-linked KEV records (1..500)")
	kevAll := flag.Bool("kev-all", false, "include all KEV records, not only ransomware-linked records")
	flag.Parse()
	if *rssLimit < 1 || *rssLimit > 25 || *kevLimit < 1 || *kevLimit > 500 {
		return fmt.Errorf("invalid bounded source limits")
	}
	if err := os.MkdirAll(filepath.Dir(*outPath), 0700); err != nil {
		return err
	}
	output, err := os.OpenFile(*outPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer output.Close()
	c := &collector{client: &http.Client{Timeout: 30 * time.Second}}
	now := time.Now().UTC().Format(time.RFC3339)
	var docs []document
	for _, spec := range []struct{ feed, source, language string }{{certEUFeed, "cert-eu-cti", "en"}, {certFRFeed, "cert-fr-cti", "fr"}, {cisaFeed, "cisa-advisory", "en"}, {sansFeed, "sans-isc-diary", "en"}} {
		batch, collectErr := collectRSS(c, spec.feed, spec.source, spec.language, now, *rssLimit)
		if collectErr != nil {
			return collectErr
		}
		docs = append(docs, batch...)
	}
	kev, version, err := collectKEV(c, now, *kevLimit, *kevAll)
	if err != nil {
		return err
	}
	docs = append(docs, kev...)
	enc := json.NewEncoder(output)
	counts := map[string]int{}
	seen := map[string]bool{}
	for _, doc := range docs {
		if seen[doc.Group] {
			continue
		}
		seen[doc.Group] = true
		if err = enc.Encode(doc); err != nil {
			return err
		}
		counts[doc.Source]++
	}
	if err = output.Sync(); err != nil {
		return err
	}
	summary := map[string]any{"retained": len(seen), "counts": counts, "requests": c.requests, "kev_catalog_version": version, "unscored": true, "automatic_labels": false, "production_writes": false}
	summaryFile, err := os.OpenFile(*outPath+".summary.json", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer summaryFile.Close()
	if err = json.NewEncoder(summaryFile).Encode(summary); err != nil {
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
