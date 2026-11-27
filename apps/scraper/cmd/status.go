package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/pkg/contract"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

func serviceHandler(rdb *redis.Client, mongoClient *mongo.Client) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
		defer cancel()
		if rdb.Ping(ctx).Err() != nil || mongoClient.Ping(ctx, readpref.Primary()) != nil {
			http.Error(w, "dependencies unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
		defer cancel()
		body, err := queueMetrics(ctx, rdb, time.Now().Unix())
		if err != nil {
			http.Error(w, "queue metrics unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = fmt.Fprint(w, body)
	})
	return mux
}

func queueMetrics(ctx context.Context, rdb *redis.Client, now int64) (string, error) {
	pipe := rdb.Pipeline()
	dueAt := strconv.FormatInt(now, 10)
	crawlerReady := pipe.SCard(ctx, contract.KeyURLsToCrawl)
	crawlerDue := pipe.ZCount(ctx, contract.KeyCrawlerScheduled, "-inf", dueAt)
	crawlerFuture := pipe.ZCount(ctx, contract.KeyCrawlerScheduled, "("+dueAt, "+inf")
	crawlerOldest := pipe.ZRangeWithScores(ctx, contract.KeyCrawlerScheduled, 0, 0)
	crawlerInFlight := pipe.ZCard(ctx, contract.KeyCrawlerInflight)
	scraperPending := pipe.ZCard(ctx, contract.KeyScraperPending)
	scraperLeases := pipe.ZCard(ctx, contract.KeyScraperInflight)
	scraperRetry := pipe.ZCard(ctx, "scraper:retry_due")
	scraperDue := pipe.ZCount(ctx, "scraper:retry_due", "-inf", dueAt)
	scraperDead := pipe.ZCard(ctx, "scraper:dead")
	crawlerFields := []string{contract.KeyURLsQueued, "deferred_total", "budget_deferred_total", "scheduled_promoted_total", "fetch_success_total", "fetch_failure_total", "infrastructure_retry_total"}
	scraperFields := []string{"claimed", "recovered", "released", "success", "transient", "permanent", "blocked", "infrastructure", "deferred"}
	crawlerStats := pipe.HMGet(ctx, contract.KeyCrawlerStats, crawlerFields...)
	scraperStats := pipe.HMGet(ctx, "scraper:stats", scraperFields...)
	if _, err := pipe.Exec(ctx); err != nil {
		return "", err
	}
	oldestAge := int64(0)
	if oldest := crawlerOldest.Val(); len(oldest) > 0 {
		oldestAge = max(0, now-int64(oldest[0].Score))
	}

	var b strings.Builder
	gauge := func(name string, value int64) {
		fmt.Fprintf(&b, "# TYPE %s gauge\n%s %d\n", name, name, value)
	}
	gauge("bwt_crawler_ready", crawlerReady.Val())
	gauge("bwt_crawler_due", crawlerDue.Val())
	gauge("bwt_crawler_future", crawlerFuture.Val())
	gauge("bwt_crawler_oldest_due_seconds", oldestAge)
	gauge("bwt_crawler_inflight", crawlerInFlight.Val())
	gauge("bwt_scraper_pending", scraperPending.Val())
	gauge("bwt_scraper_inflight", scraperLeases.Val())
	gauge("bwt_scraper_retry", scraperRetry.Val())
	gauge("bwt_scraper_retry_due", scraperDue.Val())
	gauge("bwt_scraper_quarantined", scraperDead.Val())

	counters := func(name string, keys []string, values []interface{}) {
		fmt.Fprintf(&b, "# TYPE %s counter\n", name)
		for i, key := range keys {
			var n int64
			if value, ok := values[i].(string); ok {
				n, _ = strconv.ParseInt(value, 10, 64)
			}
			fmt.Fprintf(&b, "%s{event=%q} %d\n", name, key, n)
		}
	}
	counters("bwt_crawler_events_total", crawlerFields, crawlerStats.Val())
	counters("bwt_scraper_events_total", scraperFields, scraperStats.Val())
	return b.String(), nil
}
