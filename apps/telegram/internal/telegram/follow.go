package telegram

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// Page contains metadata for every visible post, including media-only posts.
// Only text articles are sent to the storage sink.
type Page struct {
	State                    string
	Articles                 []Article
	MinID, MaxID, NextBefore int64
}

// PageError means one channel's preview could not be traversed safely.
type PageError struct{ Err error }

func (e *PageError) Error() string { return e.Err.Error() }
func (e *PageError) Unwrap() error { return e.Err }

func ParsePage(body []byte, channel string) (Page, error) {
	state, articles, err := ParsePreview(body, channel, 0)
	page := Page{State: state, Articles: articles}
	if err != nil || state == "blocked_content" {
		return page, err
	}
	canonical, kind, err := Normalize(channel)
	if err != nil || kind != "public_candidate" {
		return page, fmt.Errorf("invalid channel")
	}
	name := strings.TrimPrefix(canonical, "https://t.me/")
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return page, err
	}
	malformed := false
	doc.Find(".tgme_widget_message[data-post]").Each(func(_ int, s *goquery.Selection) {
		post, _ := s.Attr("data-post")
		parts := strings.Split(post, "/")
		if len(parts) != 2 || !strings.EqualFold(parts[0], name) {
			return
		}
		id, e := strconv.ParseInt(parts[1], 10, 64)
		if e != nil || id <= 0 {
			malformed = true
			return
		}
		if page.MinID == 0 || id < page.MinID {
			page.MinID = id
		}
		if id > page.MaxID {
			page.MaxID = id
		}
		if strings.TrimSpace(s.Find(".tgme_widget_message_text").Text()) != "" {
			stamp, _ := s.Find("time[datetime]").First().Attr("datetime")
			if _, e := time.Parse(time.RFC3339, stamp); e != nil {
				malformed = true
			}
		}
	})
	if malformed {
		return page, fmt.Errorf("malformed message; cursor not advanced")
	}
	// The public feed's older-page control is data, never a URL to follow blindly.
	doc.Find(".tme_messages_more").Each(func(_ int, s *goquery.Selection) {
		raw, ok := s.Attr("data-before")
		if !ok {
			// Older pages can include a second control for newer posts.
			if _, newer := s.Attr("data-after"); newer {
				return
			}
			href, _ := s.Attr("href")
			u, e := url.Parse(href)
			if e != nil {
				malformed = true
				return
			}
			base, _ := url.Parse("https://t.me/s/" + name)
			u = base.ResolveReference(u)
			if u.Scheme != "https" || u.Host != "t.me" || !strings.EqualFold(u.Path, base.Path) || u.User != nil {
				malformed = true
				return
			}
			query := u.Query()
			if !query.Has("before") && query.Has("after") {
				return
			}
			raw = query.Get("before")
		}
		before, e := strconv.ParseInt(raw, 10, 64)
		if e != nil || before <= 0 || page.MinID == 0 || before > page.MinID {
			malformed = true
			return
		}
		page.NextBefore = page.MinID
	})
	if malformed {
		return page, fmt.Errorf("invalid pagination control; cursor not advanced")
	}
	// Empty shells are not proof that history has been exhausted.
	if page.MaxID == 0 {
		return page, fmt.Errorf("no usable posts; history end not confirmed")
	}
	sort.Slice(page.Articles, func(i, j int) bool { return page.Articles[i].MessageID < page.Articles[j].MessageID })
	return page, nil
}

type FollowOptions struct{ MaxPages int }
type FollowResult struct {
	Pages, Stored int
	Complete      bool
}
type ArticleSink func(context.Context, Article) error

// Follow walks newest-to-oldest until the committed watermark is reached.
// BeforeID and CatchupHead persist an unfinished bounded catch-up. The watermark
// is committed only after the full interval is durably stored. Replay is safe
// because the sink upserts each message with a stable identity.
func Follow(ctx context.Context, client *http.Client, c *Candidate, opts FollowOptions, sink ArticleSink, checkpoint func() error, wait func(context.Context) error) (FollowResult, error) {
	result := FollowResult{}
	if opts.MaxPages < 1 || sink == nil {
		return result, fmt.Errorf("follow requires a positive page budget and durable storage")
	}
	canonical, kind, err := Normalize(c.URL)
	if err != nil || kind != "public_candidate" || canonical != c.URL {
		return result, fmt.Errorf("invalid channel")
	}
	if c.BeforeID < 0 || c.CatchupHead < 0 || c.LastMessageID < 0 || (c.BeforeID > 0 && c.CatchupHead == 0) {
		return result, fmt.Errorf("invalid cursor")
	}
	for result.Pages < opts.MaxPages {
		if wait != nil {
			if err = wait(ctx); err != nil {
				return result, err
			}
		}
		address := "https://t.me/s/" + strings.TrimPrefix(c.URL, "https://t.me/")
		before := c.BeforeID
		if before > 0 {
			address += "?before=" + strconv.FormatInt(before, 10)
		}
		now := time.Now().UTC()
		c.FollowedAt = &now
		body, e := Fetch(ctx, client, address)
		if e != nil {
			c.State = "check_failed"
			c.Error = e.Error()
			var status *HTTPStatusError
			if errors.As(e, &status) && status.Code >= 300 && status.Code < 400 && (status.Location == c.URL || status.Location == c.URL+"/") {
				c.State = "preview_unavailable"
				return result, nil
			}
			return result, e
		}
		page, e := ParsePage(body, c.URL)
		result.Pages++
		c.State = page.State
		c.Error = ""
		c.Messages = len(page.Articles)
		if e != nil {
			c.State = "check_failed"
			c.Error = e.Error()
			return result, &PageError{Err: e}
		}
		if page.State == "blocked_content" {
			return result, nil
		}
		if before > 0 && page.MaxID >= before {
			c.State = "check_failed"
			c.Error = "pagination did not move backwards"
			return result, &PageError{Err: errors.New(c.Error)}
		}
		head := c.CatchupHead
		if head == 0 {
			head = page.MaxID
		}
		for _, article := range page.Articles {
			if article.MessageID <= c.LastMessageID || article.MessageID > head {
				continue
			}
			if err = sink(ctx, article); err != nil {
				return result, err
			}
			result.Stored++
		}
		// All sink writes succeeded. Only now persist pagination progress.
		done := page.NextBefore == 0 || page.MinID <= c.LastMessageID || page.NextBefore <= c.LastMessageID
		if done {
			if head > c.LastMessageID {
				c.LastMessageID = head
			}
			c.CatchupHead = 0
			c.BeforeID = 0
			result.Complete = true
		} else {
			c.CatchupHead = head
			c.BeforeID = page.NextBefore
		}
		c.Tracked = true
		c.State = "web_readable"
		c.Failures = 0
		if checkpoint != nil {
			if err = checkpoint(); err != nil {
				return result, err
			}
		}
		if done {
			return result, nil
		}
	}
	return result, nil
}

func Due(c Candidate, mode string, now time.Time) bool {
	if c.Kind != "public_candidate" || c.State == "blocked_content" || (c.NextCheckAt != nil && c.NextCheckAt.After(now)) {
		return false
	}
	tracked := c.Tracked || c.State == "web_readable"
	return (mode == "follow" && tracked) || (mode == "discover" && !tracked)
}

// DeferCheck bounds retries and keeps temporarily unavailable tracked channels
// in the follow queue instead of mixing them back into discovery.
func DeferCheck(c *Candidate, success bool, interval time.Duration, now time.Time) {
	if success {
		c.Failures = 0
	} else {
		c.Failures++
		for i := 1; i < c.Failures && interval < 24*time.Hour; i++ {
			interval *= 2
		}
		if interval > 24*time.Hour {
			interval = 24 * time.Hour
		}
	}
	next := now.Add(interval)
	c.NextCheckAt = &next
}
