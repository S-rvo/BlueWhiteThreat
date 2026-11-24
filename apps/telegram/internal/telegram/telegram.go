// Package telegram imports CTI seeds and reads unauthenticated Telegram previews.
package telegram

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/S-rvo/BlueWhiteThreat/pkg/csamfilter"
)

var Sources = []string{
	"https://raw.githubusercontent.com/fastfire/deepdarkCTI/main/telegram_infostealer.md",
	"https://raw.githubusercontent.com/fastfire/deepdarkCTI/main/telegram_threat_actors.md",
}

// Provenance keeps upstream claims separate from observations made locally.
type Provenance struct {
	Source     string `json:"source"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Category   string `json:"category"`
	AttackType string `json:"attack_type,omitempty"`
}
type Candidate struct {
	StorageID     string       `json:"storage_id,omitempty"`
	Tracked       bool         `json:"tracked,omitempty"`
	LastMessageID int64        `json:"last_message_id,omitempty"`
	CatchupHead   int64        `json:"catchup_head,omitempty"`
	BeforeID      int64        `json:"before_id,omitempty"`
	FollowedAt    *time.Time   `json:"followed_at,omitempty"`
	NextCheckAt   *time.Time   `json:"next_check_at,omitempty"`
	Failures      int          `json:"failures,omitempty"`
	URL           string       `json:"url"`
	Kind          string       `json:"kind"`
	Provenance    []Provenance `json:"provenance"`
	State         string       `json:"state"`
	CheckedAt     *time.Time   `json:"checked_at,omitempty"`
	Messages      int          `json:"visible_messages"`
	Error         string       `json:"error,omitempty"`
}
type Article struct {
	URL         string    `bson:"url" json:"url"`
	Source      string    `bson:"source" json:"source"`
	Channel     string    `bson:"channel" json:"channel"`
	MessageID   int64     `bson:"message_id" json:"message_id"`
	PublishedAt time.Time `bson:"published_at" json:"published_at"`
	ScrapedAt   time.Time `bson:"scraped_at" json:"scraped_at"`
	Content     string    `bson:"content" json:"content"`
	Links       []string  `bson:"telegram_links" json:"telegram_links"`
}

var username = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{3,31}$`)
var invite = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
var link = regexp.MustCompile(`https?://(?:t\.me|telegram\.me)/[^\s<>\[\]()|]+`)

// Normalize accepts only known Telegram destinations, never arbitrary URLs.
func Normalize(raw string) (string, string, error) {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(raw), ".,;"))
	if err != nil {
		return "", "", fmt.Errorf("invalid URL")
	}
	host := strings.ToLower(u.Host)
	if (u.Scheme != "https" && u.Scheme != "http") || (host != "t.me" && host != "telegram.me") || u.User != nil {
		return "", "", fmt.Errorf("unsupported destination")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) == 0 {
		return "", "", fmt.Errorf("missing destination")
	}
	if parts[0] == "s" {
		parts = parts[1:]
	}
	if len(parts) == 0 {
		return "", "", fmt.Errorf("missing destination")
	}
	if strings.HasPrefix(parts[0], "+") && len(parts) == 1 && invite.MatchString(parts[0][1:]) {
		return "https://t.me/" + parts[0], "invite", nil
	}
	if parts[0] == "joinchat" && len(parts) == 2 && invite.MatchString(parts[1]) {
		return "https://t.me/+" + parts[1], "invite", nil
	}
	if !username.MatchString(parts[0]) || len(parts) > 2 {
		return "", "", fmt.Errorf("unsupported path")
	}
	switch strings.ToLower(parts[0]) {
	case "joinchat", "share", "proxy", "socks", "login", "addstickers", "addemoji", "iv", "c":
		return "", "", fmt.Errorf("reserved path")
	}
	if len(parts) == 2 {
		if id, e := strconv.ParseInt(parts[1], 10, 64); e != nil || id <= 0 {
			return "", "", fmt.Errorf("invalid post")
		}
	}
	return "https://t.me/" + strings.ToLower(parts[0]), "public_candidate", nil
}

func ParseSeeds(r io.Reader, source string) ([]Candidate, error) {
	out := []Candidate{}
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 4096), 1024*1024)
	for s.Scan() {
		cells := strings.Split(strings.Trim(strings.TrimSpace(s.Text()), "|"), "|")
		if len(cells) < 3 {
			continue
		}
		raw := link.FindString(cells[0])
		if raw == "" {
			continue
		}
		normalized, kind, err := Normalize(raw)
		if err != nil {
			continue
		}
		category := "threat_actor"
		if strings.Contains(source, "infostealer") {
			category = "infostealer"
		}
		p := Provenance{Source: source, Name: strings.TrimSpace(cells[2]), Status: strings.TrimSpace(cells[1]), Category: category}
		if len(cells) > 3 {
			p.AttackType = strings.TrimSpace(cells[3])
		}
		state := "pending"
		if kind == "invite" {
			state = "invitation_required"
		}
		if csamfilter.IsBlockedURL(normalized) || csamfilter.ContainsBlockedContent(p.Name) {
			continue
		}
		out = append(out, Candidate{URL: normalized, Kind: kind, State: state, Provenance: []Provenance{p}})
	}
	return out, s.Err()
}

func Merge(groups ...[]Candidate) []Candidate {
	out := []Candidate{}
	indices := map[string]int{}
	for _, group := range groups {
		for _, c := range group {
			if i, ok := indices[c.URL]; ok {
				for _, p := range c.Provenance {
					found := false
					for _, old := range out[i].Provenance {
						if old == p {
							found = true
							break
						}
					}
					if !found {
						out[i].Provenance = append(out[i].Provenance, p)
					}
				}
			} else {
				indices[c.URL] = len(out)
				out = append(out, c)
			}
		}
	}
	return out
}

// HTTPStatusError preserves a safe redirect destination without following it.
type HTTPStatusError struct {
	Code     int
	Location string
}

func (e *HTTPStatusError) Error() string {
	if e.Location != "" {
		return fmt.Sprintf("HTTP %d (redirect to %s; not followed)", e.Code, e.Location)
	}
	return fmt.Sprintf("HTTP %d", e.Code)
}

// Fetch bounds memory, request duration (via client), and redirects. No cookies or credentials.
func Fetch(ctx context.Context, client *http.Client, address string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Telegram/0.1")
	safe := *client
	safe.Jar = nil
	safe.CheckRedirect = func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := safe.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		status := &HTTPStatusError{Code: resp.StatusCode}
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			if location, e := resp.Location(); e == nil {
				location.User = nil
				location.RawQuery = ""
				location.Fragment = ""
				status.Location = location.String()
			}
		}
		return nil, status
	}
	const max = 4 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, err
	}
	if len(body) > max {
		return nil, fmt.Errorf("response exceeds 4 MiB")
	}
	return body, nil
}

func ParsePreview(body []byte, channel string, limit int) (string, []Article, error) {
	canonical, kind, err := Normalize(channel)
	if err != nil || kind != "public_candidate" {
		return "unknown", nil, fmt.Errorf("invalid channel")
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return "unknown", nil, err
	}
	// Gate all visible text before extracting or persisting any content.
	if csamfilter.IsBlockedURL(canonical) || csamfilter.ContainsBlockedContent(doc.Text()) {
		return "blocked_content", nil, nil
	}
	name := strings.TrimPrefix(canonical, "https://t.me/")
	articles := []Article{}
	seen := map[int64]bool{}
	validPosts := 0
	doc.Find(".tgme_widget_message[data-post]").Each(func(_ int, s *goquery.Selection) {
		post, _ := s.Attr("data-post")
		parts := strings.Split(post, "/")
		if len(parts) != 2 || !strings.EqualFold(parts[0], name) {
			return
		}
		id, e := strconv.ParseInt(parts[1], 10, 64)
		if e != nil || id <= 0 || seen[id] {
			return
		}
		seen[id] = true
		validPosts++
		contentNode := s.Find(".tgme_widget_message_text").First().Clone()
		contentNode.Find("br").ReplaceWithHtml("\n")
		content := strings.TrimSpace(contentNode.Text())
		if content == "" {
			return
		}
		stamp, _ := s.Find("time[datetime]").First().Attr("datetime")
		published, e := time.Parse(time.RFC3339, stamp)
		if e != nil {
			return
		}
		links := []string{}
		linkSeen := map[string]bool{}
		add := func(raw string) {
			normalized, _, e := Normalize(raw)
			if e == nil && normalized != canonical && !linkSeen[normalized] && !csamfilter.IsBlockedURL(normalized) {
				linkSeen[normalized] = true
				links = append(links, normalized)
			}
		}
		s.Find(".tgme_widget_message_text a[href], .tgme_widget_message_forwarded_from[href]").Each(func(_ int, a *goquery.Selection) { href, _ := a.Attr("href"); add(href) })
		for _, raw := range link.FindAllString(content, -1) {
			add(raw)
		}
		articles = append(articles, Article{URL: fmt.Sprintf("%s/%d", canonical, id), Source: "telegram", Channel: canonical, MessageID: id, PublishedAt: published, ScrapedAt: time.Now().UTC(), Content: content, Links: links})
	})
	if validPosts == 0 {
		return "preview_unavailable", articles, nil
	}
	if limit > 0 && len(articles) > limit {
		articles = articles[len(articles)-limit:]
	}
	return "web_readable", articles, nil
}

func Probe(ctx context.Context, client *http.Client, c *Candidate, limit int) ([]Article, error) {
	if c.Kind != "public_candidate" {
		return nil, nil
	}
	now := time.Now().UTC()
	c.CheckedAt = &now
	c.Error = ""
	c.Messages = 0
	body, err := Fetch(ctx, client, "https://t.me/s/"+strings.TrimPrefix(c.URL, "https://t.me/"))
	if err != nil {
		c.State = "check_failed"
		c.Error = err.Error()
		var status *HTTPStatusError
		if errors.As(err, &status) && status.Code >= 300 && status.Code < 400 {
			// A redirect from /s/name to /name is a landing page, not a feed.
			if status.Location == c.URL || status.Location == c.URL+"/" {
				c.State = "preview_unavailable"
				return nil, nil
			}
		}
		return nil, err
	}
	state, articles, err := ParsePreview(body, c.URL, limit)
	c.State = state
	c.Messages = len(articles)
	return articles, err
}
