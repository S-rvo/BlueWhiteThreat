package telegram

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func feed(ids []int, next int) string {
	body := "<div class='tgme_channel_history'>"
	for _, id := range ids {
		body += post("examplechannel", id, fmt.Sprintf("message %d", id))
	}
	if next > 0 {
		body += fmt.Sprintf(`<a class="tme_messages_more" data-before="%d" href="/s/examplechannel?before=%d">more</a>`, next, next)
	}
	return body + "</div>"
}
func pageClient(pages map[string]string, requests *[]string) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		key := r.URL.Query().Get("before")
		*requests = append(*requests, key)
		body, ok := pages[key]
		if !ok {
			return nil, fmt.Errorf("unexpected page %q", key)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
}
func TestFollowResumesBoundedCatchupAndDoesNotSkipNewArrivals(t *testing.T) {
	requests := []string{}
	pages := map[string]string{"": feed([]int{8, 9, 10}, 8), "8": feed([]int{5, 6, 7}, 5), "5": feed([]int{2, 3, 4}, 2), "2": feed([]int{1}, 0)}
	client := pageClient(pages, &requests)
	c := Candidate{URL: "https://t.me/examplechannel", Kind: "public_candidate", Tracked: true, LastMessageID: 3}
	stored := map[int64]int{}
	sink := func(_ context.Context, a Article) error { stored[a.MessageID]++; return nil }
	checkpoints := 0
	checkpoint := func() error { checkpoints++; return nil }
	r, e := Follow(context.Background(), client, &c, FollowOptions{MaxPages: 1}, sink, checkpoint, nil)
	if e != nil || r.Complete || c.LastMessageID != 3 || c.BeforeID != 8 || c.CatchupHead != 10 {
		t.Fatalf("first=%+v candidate=%+v err=%v", r, c, e)
	}
	// New messages arrive while old catch-up is pending. They must not be skipped.
	pages[""] = feed([]int{10, 11, 12}, 10)
	r, e = Follow(context.Background(), client, &c, FollowOptions{MaxPages: 3}, sink, checkpoint, nil)
	if e != nil || !r.Complete || c.LastMessageID != 10 || c.BeforeID != 0 {
		t.Fatalf("resume=%+v candidate=%+v err=%v", r, c, e)
	}
	for id := int64(4); id <= 10; id++ {
		if stored[id] != 1 {
			t.Errorf("message %d writes=%d", id, stored[id])
		}
	}
	r, e = Follow(context.Background(), client, &c, FollowOptions{MaxPages: 3}, sink, checkpoint, nil)
	if e != nil || !r.Complete || c.LastMessageID != 12 || stored[11] != 1 || stored[12] != 1 {
		t.Fatalf("new=%+v candidate=%+v err=%v", r, c, e)
	}
	if strings.Join(requests, ",") != ",8,5," || checkpoints != 4 {
		t.Fatalf("requests=%v checkpoints=%d", requests, checkpoints)
	}
}
func TestFollowInitialBackfillAndSinkFailureReplay(t *testing.T) {
	requests := []string{}
	client := pageClient(map[string]string{"": feed([]int{3, 4}, 3), "3": feed([]int{1, 2}, 0)}, &requests)
	c := Candidate{URL: "https://t.me/examplechannel", Kind: "public_candidate"}
	failed := true
	writes := map[int64]int{}
	sink := func(_ context.Context, a Article) error {
		if a.MessageID == 4 && failed {
			return errors.New("disk full")
		}
		writes[a.MessageID]++
		return nil
	}
	_, err := Follow(context.Background(), client, &c, FollowOptions{MaxPages: 2}, sink, nil, nil)
	if err == nil || c.LastMessageID != 0 || c.BeforeID != 0 || c.CatchupHead != 0 {
		t.Fatalf("unsafe cursor after storage failure: %+v %v", c, err)
	}
	failed = false
	r, err := Follow(context.Background(), client, &c, FollowOptions{MaxPages: 2}, sink, nil, nil)
	if err != nil || !r.Complete || c.LastMessageID != 4 || len(writes) != 4 {
		t.Fatalf("replay=%+v %+v %v", r, c, err)
	}
}
func TestFollowNetworkFailureKeepsPageCheckpoint(t *testing.T) {
	requests := []string{}
	client := pageClient(map[string]string{"": feed([]int{4, 5}, 4)}, &requests)
	c := Candidate{URL: "https://t.me/examplechannel", Kind: "public_candidate"}
	_, err := Follow(context.Background(), client, &c, FollowOptions{MaxPages: 3}, func(context.Context, Article) error { return nil }, func() error { return nil }, nil)
	if err == nil || c.LastMessageID != 0 || c.BeforeID != 4 || c.CatchupHead != 5 {
		t.Fatalf("lost resumable range: %+v %v", c, err)
	}
}
func TestFollowRejectsStalledPagination(t *testing.T) {
	requests := []string{}
	client := pageClient(map[string]string{"4": feed([]int{4, 5}, 4)}, &requests)
	c := Candidate{URL: "https://t.me/examplechannel", Kind: "public_candidate", BeforeID: 4, CatchupHead: 5}
	r, err := Follow(context.Background(), client, &c, FollowOptions{MaxPages: 5}, func(context.Context, Article) error { t.Fatal("should not store stalled page"); return nil }, nil, nil)
	if err == nil || r.Pages != 1 || c.BeforeID != 4 {
		t.Fatalf("%+v %+v %v", r, c, err)
	}
}
func TestParsePageMediaAndInvalidMetadata(t *testing.T) {
	body := feed([]int{1, 2}, 0) + post("examplechannel", 3, "")
	p, e := ParsePage([]byte(body), "https://t.me/examplechannel")
	if e != nil || p.MaxID != 3 || len(p.Articles) != 2 {
		t.Fatalf("%+v %v", p, e)
	}
	for _, body := range []string{
		"<html>landing</html>",
		strings.Replace(feed([]int{1}, 0), "2026-10-23T10:00:00+00:00", "bad-date", 1),
		feed([]int{1}, 0) + `<a class="tme_messages_more" href="https://evil.example/?before=1">more</a>`,
	} {
		if _, e = ParsePage([]byte(body), "https://t.me/examplechannel"); e == nil {
			t.Fatalf("accepted invalid preview %s", body)
		}
	}
}
func TestDueAndBackoff(t *testing.T) {
	now := time.Now()
	c := Candidate{Kind: "public_candidate", State: "web_readable"}
	if !Due(c, "follow", now) || Due(c, "discover", now) {
		t.Fatal("readable channel in wrong queue")
	}
	c.Tracked = true
	c.State = "check_failed"
	if !Due(c, "follow", now) || Due(c, "discover", now) {
		t.Fatal("failed tracked channel lost its queue")
	}
	DeferCheck(&c, false, time.Minute, now)
	if Due(c, "follow", now) {
		t.Fatal("backoff ignored")
	}
	DeferCheck(&c, false, time.Minute, now)
	if !c.NextCheckAt.Equal(now.Add(2 * time.Minute)) {
		t.Fatal(c.NextCheckAt)
	}
	DeferCheck(&c, true, time.Minute, now)
	if c.Failures != 0 {
		t.Fatal(c.Failures)
	}
}

func TestFollowBlockedPageDoesNotAdvanceCursor(t *testing.T) {
	requests := []string{}
	client := pageClient(map[string]string{"": post("examplechannel", 10, "child porn")}, &requests)
	c := Candidate{URL: "https://t.me/examplechannel", Kind: "public_candidate", Tracked: true, LastMessageID: 8}
	result, err := Follow(context.Background(), client, &c, FollowOptions{MaxPages: 2}, func(context.Context, Article) error { t.Fatal("blocked text reached storage"); return nil }, nil, nil)
	if err != nil || result.Stored != 0 || c.State != "blocked_content" || c.LastMessageID != 8 {
		t.Fatalf("%+v %+v %v", result, c, err)
	}
}

func TestFollowResumesWithBothPaginationDirections(t *testing.T) {
	requests := []string{}
	newer := `<a class="tme_messages_more js-messages_more" href="/s/examplechannel?after=764" data-after="764">newer</a>`
	client := pageClient(map[string]string{
		"765": feed([]int{742, 743, 764}, 742) + newer,
		"742": feed([]int{739, 741}, 0) + `<a class="tme_messages_more" href="/s/examplechannel?after=741">newer</a>`,
	}, &requests)
	c := Candidate{URL: "https://t.me/examplechannel", Kind: "public_candidate", Tracked: true, BeforeID: 765, CatchupHead: 780, LastMessageID: 740}
	var stored []int64
	r, err := Follow(context.Background(), client, &c, FollowOptions{MaxPages: 2}, func(_ context.Context, a Article) error {
		stored = append(stored, a.MessageID)
		return nil
	}, nil, nil)
	if err != nil || !r.Complete || c.BeforeID != 0 || c.LastMessageID != 780 {
		t.Fatalf("resume: pages=%d cursor=%d before=%d error=%v", r.Pages, c.LastMessageID, c.BeforeID, err)
	}
	if fmt.Sprint(requests) != "[765 742]" || fmt.Sprint(stored) != "[742 743 764 741]" {
		t.Fatalf("requests=%v writes=%v", requests, stored)
	}
}
