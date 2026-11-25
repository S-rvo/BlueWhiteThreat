package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"time"
)

type publicFeed struct {
	id, endpoint, format string
}

var publicFeeds = []publicFeed{
	{"deepdarkcti", "https://api.github.com/repos/fastfire/deepdarkCTI", "github_repository"},
	{"cert-fr-cti", certFRFeed, "rss"},
	{"cert-fr-alerts", "https://cert.cyber.gouv.fr/alerte/feed/", "rss"},
	{"cert-eu-cti", certEUFeed, "rss"},
	{"cisa-kev", "https://raw.githubusercontent.com/cisagov/kev-data/develop/known_exploited_vulnerabilities.json", "kev"},
	{"sans-isc-diary", sansFeed, "rss"},
}

type feedStatus struct {
	Source    string     `json:"source"`
	Format    string     `json:"format"`
	Status    string     `json:"status"`
	Entries   int        `json:"entries,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
	Error     string     `json:"error,omitempty"`
}

func writeFeedStatus(out io.Writer, feeds []publicFeed, now time.Time) error {
	c := &collector{client: &http.Client{
		Timeout: 8 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
	rows := make([]feedStatus, 0, len(feeds))
	failures := 0
	for _, feed := range feeds {
		row := feedStatus{Source: feed.id, Format: feed.format, Status: "ok"}
		body, _, err := c.get(feed.endpoint)
		if err == nil {
			var updated time.Time
			row.Entries, updated, err = feedMetadata(feed.format, body)
			if err == nil {
				updated = updated.UTC()
				row.UpdatedAt = &updated
			}
		}
		if err != nil {
			row.Status, row.Error = "error", err.Error()
			failures++
		}
		rows = append(rows, row)
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(struct {
		ObservedAt time.Time    `json:"observed_at"`
		Sources    []feedStatus `json:"sources"`
	}{now.UTC(), rows}); err != nil {
		return err
	}
	if failures != 0 {
		return fmt.Errorf("%d/%d sources publiques en erreur", failures, len(feeds))
	}
	return nil
}

func feedMetadata(format string, body []byte) (int, time.Time, error) {
	switch format {
	case "rss":
		var feed struct {
			XMLName xml.Name `xml:"rss"`
			Items   []struct {
				Date string `xml:"pubDate"`
			} `xml:"channel>item"`
		}
		if err := xml.Unmarshal(body, &feed); err != nil {
			return 0, time.Time{}, fmt.Errorf("flux RSS invalide")
		}
		var latest time.Time
		for _, item := range feed.Items {
			raw := strings.TrimSpace(item.Date)
			// Le CERT-EU publie en CET/CEST ; net/mail interprète les
			// décalages numériques sans dépendre du fuseau de la machine.
			if cut, ok := strings.CutSuffix(raw, " CET"); ok {
				raw = cut + " +0100"
			} else if cut, ok := strings.CutSuffix(raw, " CEST"); ok {
				raw = cut + " +0200"
			}
			if date, err := mail.ParseDate(raw); err == nil && date.After(latest) {
				latest = date
			}
		}
		if latest.IsZero() {
			return 0, time.Time{}, fmt.Errorf("aucune publication RSS datée")
		}
		return len(feed.Items), latest, nil
	case "kev":
		var feed struct {
			Date  time.Time         `json:"dateReleased"`
			Count int               `json:"count"`
			Items []json.RawMessage `json:"vulnerabilities"`
		}
		if err := json.Unmarshal(body, &feed); err != nil || feed.Date.IsZero() ||
			feed.Count == 0 || feed.Count != len(feed.Items) {
			return 0, time.Time{}, fmt.Errorf("catalogue KEV invalide")
		}
		return feed.Count, feed.Date, nil
	case "github_repository":
		var repo struct {
			Name     string    `json:"full_name"`
			PushedAt time.Time `json:"pushed_at"`
		}
		if err := json.Unmarshal(body, &repo); err != nil || repo.PushedAt.IsZero() ||
			!strings.EqualFold(repo.Name, "fastfire/deepdarkCTI") {
			return 0, time.Time{}, fmt.Errorf("métadonnées DeepDarkCTI invalides")
		}
		return 0, repo.PushedAt, nil
	default:
		return 0, time.Time{}, fmt.Errorf("format de source non reconnu")
	}
}
