package main

import (
	"context"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestQueueMetricsTracksDueFutureAndConfirmedOutcomes(t *testing.T) {
	r := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	t.Cleanup(func() { _ = r.Close() })
	ctx := context.Background()
	if err := r.ZAdd(ctx, "crawler:scheduled", redis.Z{Score: 700, Member: "a"}, redis.Z{Score: 1500, Member: "b"}).Err(); err != nil {
		t.Fatal(err)
	}
	if err := r.HSet(ctx, "scraper:stats", "success", 14, "infrastructure", 2).Err(); err != nil {
		t.Fatal(err)
	}
	body, err := queueMetrics(ctx, r, 1000)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"bwt_crawler_due 1", "bwt_crawler_future 1", "bwt_crawler_oldest_due_seconds 300",
		"bwt_scraper_events_total{event=\"success\"} 14", "bwt_scraper_events_total{event=\"infrastructure\"} 2"} {
		if !strings.Contains(body, line+"\n") {
			t.Errorf("missing %q", line)
		}
	}
}
