package sites

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/db"
)

func TestScrapeWorkerContinuesAfterEmptyQueueAndDependencyOutages(t *testing.T) {
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	var next, resets int
	var outcomes []db.ScrapeOutcome
	var delays []time.Duration
	runScrapeWorker(ctx, workerActions{
		claim: func(context.Context) (db.ScrapeClaim, error) {
			next++
			switch next {
			case 1:
				return db.ScrapeClaim{}, errors.New("fixture Redis unavailable")
			case 2:
				return db.ScrapeClaim{}, nil
			default:
				return db.ScrapeClaim{URL: "http://fixture.test/"}, nil
			}
		},
		scrape: func(string) (error, bool) {
			if next <= 5 {
				return errScrapeBrowser, false
			}
			return nil, false
		},
		finish: func(_ context.Context, _ db.ScrapeClaim, out db.ScrapeOutcome) error {
			outcomes = append(outcomes, out)
			if out == db.ScrapeSuccess {
				stop()
			}
			return nil
		},
		resetBrowser: func() { resets++ },
	}, func(_ context.Context, delay time.Duration) bool {
		delays = append(delays, delay)
		return len(delays) <= 5
	})
	if want := []db.ScrapeOutcome{db.ScrapeInfrastructure, db.ScrapeInfrastructure, db.ScrapeInfrastructure, db.ScrapeSuccess}; !slices.Equal(outcomes, want) {
		t.Fatalf("outcomes: %v", outcomes)
	}
	if resets != 1 || !slices.Equal(delays, []time.Duration{2 * time.Second, 500 * time.Millisecond, 5 * time.Second}) {
		t.Fatalf("resets=%d delays=%v", resets, delays)
	}
}

func TestScrapeWorkerReleasesCancelledJobWithLiveCleanupContext(t *testing.T) {
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	released := false
	runScrapeWorker(ctx, workerActions{
		claim: func(context.Context) (db.ScrapeClaim, error) {
			return db.ScrapeClaim{URL: "http://fixture.test/", Token: "current"}, nil
		},
		scrape: func(string) (error, bool) {
			stop()
			return context.Canceled, false
		},
		release: func(ctx context.Context, claim db.ScrapeClaim) error {
			released = true
			if ctx.Err() != nil || claim.Token != "current" {
				t.Fatalf("invalid cleanup: %v %v", ctx.Err(), claim)
			}
			return nil
		},
		finish: func(context.Context, db.ScrapeClaim, db.ScrapeOutcome) error {
			t.Fatal("cancelled job acknowledged")
			return nil
		},
	}, func(context.Context, time.Duration) bool { return false })
	if !released {
		t.Fatal("cancelled lease not released")
	}
}
