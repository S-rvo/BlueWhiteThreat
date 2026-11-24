package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	tg "github.com/S-rvo/BlueWhiteThreat/apps/telegram/internal/telegram"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type config struct {
	mode, registry, seedDir, torProxy, outbox                                  string
	maxChannels, maxMessages, maxPages, maxDiscoveryDepth, maxNewCandidates    int
	delay, timeout, interval, followInterval, discoveryInterval, retryInterval time.Duration
	store                                                                      bool
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error { return runWith(os.Args[1:], tg.NewTorClient) }
func runWith(args []string, newClient func(string) (*http.Client, error)) error {
	f := flag.NewFlagSet("telegram", flag.ContinueOnError)
	c := config{}
	torDefault := os.Getenv("TOR_PROXY_URL")
	if torDefault == "" {
		torDefault = tg.DefaultTorProxy
	}
	f.StringVar(&c.mode, "mode", "import", "import, discover, follow, or all")
	probe := f.Bool("probe", false, "Legacy alias for -mode discover")
	f.StringVar(&c.registry, "registry", "data/candidates.json", "Persistent registry")
	f.StringVar(&c.seedDir, "seed-dir", "", "Local seed directory instead of GitHub")
	f.StringVar(&c.torProxy, "tor-proxy", torDefault, "Tor SOCKS5 endpoint")
	f.StringVar(&c.outbox, "outbox-dir", "", "Durable JSON message directory (default: beside registry/outbox)")
	f.IntVar(&c.maxChannels, "max-channels", 5, "Channel budget per queue per cycle")
	f.IntVar(&c.maxMessages, "max-messages", 20, "Discovery sample size (follow stores entire pages)")
	f.IntVar(&c.maxPages, "max-pages", 10, "Page budget per followed channel per cycle")
	f.IntVar(&c.maxDiscoveryDepth, "max-discovery-depth", 2, "Maximum link hops from an imported or manual seed")
	f.IntVar(&c.maxNewCandidates, "max-new-candidates", 50, "Maximum unique candidates added from message links per cycle")
	f.DurationVar(&c.delay, "delay", 2*time.Second, "Minimum delay between Telegram requests")
	f.DurationVar(&c.timeout, "request-timeout", 60*time.Second, "Total HTTP request timeout")
	f.DurationVar(&c.interval, "interval", 0, "Repeat cycles after this delay; 0 runs once")
	f.DurationVar(&c.followInterval, "follow-interval", 10*time.Minute, "Minimum interval between follow checks")
	f.DurationVar(&c.discoveryInterval, "discovery-interval", 24*time.Hour, "Recheck delay for unavailable candidates")
	f.DurationVar(&c.retryInterval, "retry-interval", 15*time.Minute, "Initial error backoff, doubled up to 24h")
	f.BoolVar(&c.store, "store", false, "Store messages in MongoDB using MONGO_URI instead of JSON outbox")
	if err := f.Parse(args); err != nil {
		return err
	}
	if *probe {
		if c.mode != "import" && c.mode != "discover" {
			return fmt.Errorf("-probe conflicts with -mode %s", c.mode)
		}
		c.mode = "discover"
	}
	if c.mode != "import" && c.mode != "discover" && c.mode != "follow" && c.mode != "all" {
		return fmt.Errorf("invalid mode")
	}
	if f.NArg() != 0 || c.maxChannels < 1 || c.maxMessages < 1 || c.maxMessages > 100 || c.maxPages < 1 || c.maxDiscoveryDepth < 0 || c.maxNewCandidates < 0 || c.delay < 0 || c.timeout <= 0 || c.interval < 0 || c.followInterval <= 0 || c.discoveryInterval <= 0 || c.retryInterval <= 0 {
		return fmt.Errorf("invalid collection bounds")
	}
	if c.store && os.Getenv("MONGO_URI") == "" {
		return fmt.Errorf("-store requires MONGO_URI")
	}
	if c.outbox == "" {
		c.outbox = filepath.Join(filepath.Dir(c.registry), "outbox")
	}
	unlock, err := lockRegistry(c.registry)
	if err != nil {
		return err
	}
	defer unlock()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return repeat(ctx, c.interval, func() error { return runCycle(ctx, c, newClient) })
}

// Cycles never overlap. A failed cycle backs off for the same interval, through
// the same mandatory Tor transport; cancellation interrupts the wait immediately.
func repeat(ctx context.Context, interval time.Duration, cycle func() error) error {
	for {
		if ctx.Err() != nil {
			return nil
		}
		err := cycle()
		if ctx.Err() != nil {
			return nil
		}
		if interval == 0 {
			return err
		}
		if err != nil {
			log.Printf("cycle failed: %v; retry after %s", err, interval)
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

func runCycle(ctx context.Context, c config, newClient func(string) (*http.Client, error)) (runErr error) {
	client, err := newClient(c.torProxy)
	if err != nil {
		return err
	}
	client.Timeout = c.timeout
	defer client.CloseIdleConnections()
	log.Printf("mode=%s HTTP routing: SOCKS5 Tor required; destination DNS via proxy; no direct fallback", c.mode)
	var candidates []tg.Candidate
	body, err := os.ReadFile(c.registry)
	if err == nil {
		if err = json.Unmarshal(body, &candidates); err != nil {
			return fmt.Errorf("registry: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	// Migrate the old metadata-only registry without advancing any message cursor.
	for i := range candidates {
		if candidates[i].State == "web_readable" {
			candidates[i].Tracked = true
		}
		// Preserve direct local seeds even if another channel later cites them.
		if candidates[i].Kind == "public_candidate" && len(candidates[i].Provenance) == 0 {
			candidates[i].Provenance = []tg.Provenance{{Category: "manual"}}
		}
	}
	if c.mode != "follow" {
		imported := []tg.Candidate{}
		var importErr error
		for _, source := range tg.Sources {
			if c.seedDir != "" {
				body, err = os.ReadFile(filepath.Join(c.seedDir, filepath.Base(source)))
			} else {
				body, err = tg.Fetch(ctx, client, source)
			}
			if err != nil {
				importErr = fmt.Errorf("import %s: %w", filepath.Base(source), err)
				break
			}
			entries, e := tg.ParseSeeds(bytes.NewReader(body), source)
			if e != nil {
				importErr = e
				break
			}
			if len(entries) == 0 {
				importErr = fmt.Errorf("no seeds in %s", source)
				break
			}
			imported = tg.Merge(imported, entries)
		}
		if importErr != nil {
			if c.mode != "all" || len(candidates) == 0 {
				return importErr
			}
			log.Printf("seed refresh failed; continuing existing queues: %v", importErr)
		} else {
			candidates = tg.Merge(candidates, imported)
		}
	}

	checked, saved, outsideDepth := 0, 0, 0
	bounds := tg.NewDiscoveryBounds(candidates, c.maxDiscoveryDepth, c.maxNewCandidates)
	discovered := []tg.Candidate{}
	checkpoint := func() error { return saveRegistry(c.registry, tg.Merge(candidates, discovered)) }
	defer func() {
		if e := checkpoint(); e != nil {
			runErr = errors.Join(runErr, fmt.Errorf("save registry: %w", e))
			return
		}
		fmt.Printf("candidates=%d checked=%d stored=%d new_candidates=%d outside_depth=%d links_skipped_depth=%d links_skipped_budget=%d registry=%s\n",
			len(tg.Merge(candidates, discovered)), checked, saved, bounds.NewCandidates, outsideDepth, bounds.SkippedDepth, bounds.SkippedBudget, c.registry)
	}()
	if c.mode == "import" {
		return nil
	}
	var sink tg.ArticleSink
	storageID := ""
	if c.store {
		connectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		db, e := mongo.Connect(connectCtx, options.Client().ApplyURI(os.Getenv("MONGO_URI")))
		if e == nil {
			e = db.Ping(connectCtx, nil)
		}
		cancel()
		if e != nil {
			return e
		}
		defer func() {
			closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = db.Disconnect(closeCtx)
		}()
		name := os.Getenv("MONGO_DB")
		if name == "" {
			name = "BlueWhiteThreat"
		}
		collection := db.Database(name).Collection("article")
		digest := sha256.Sum256([]byte(os.Getenv("MONGO_URI") + "/" + name))
		storageID = fmt.Sprintf("mongo:%x", digest[:12])
		sink = func(ctx context.Context, a tg.Article) error {
			writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			_, e := collection.UpdateOne(writeCtx, bson.M{"_id": "telegram:" + a.URL}, bson.M{"$set": a}, options.Update().SetUpsert(true))
			return e
		}
	} else if c.mode == "follow" || c.mode == "all" {
		absolute, e := filepath.Abs(c.outbox)
		if e != nil {
			return e
		}
		storageID = "json:" + absolute
		sink = func(_ context.Context, a tg.Article) error {
			name := strings.TrimPrefix(a.Channel, "https://t.me/")
			return atomicJSON(filepath.Join(absolute, name, fmt.Sprintf("%d.json", a.MessageID)), a)
		}
	}
	discoverLinks := func(a tg.Article) {
		discovered = append(discovered, bounds.Links(a)...)
	}
	durableSink := func(ctx context.Context, a tg.Article) error {
		if sink == nil {
			return fmt.Errorf("durable storage required")
		}
		if e := sink(ctx, a); e != nil {
			return e
		}
		saved++
		discoverLinks(a)
		return nil
	}
	var lastRequest time.Time
	wait := func(ctx context.Context) error {
		delay := time.Until(lastRequest.Add(c.delay))
		if !lastRequest.IsZero() && delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		lastRequest = time.Now()
		return nil
	}
	// Separate queues; snapshot membership before discovery so newly admitted
	// sources begin following next cycle and cannot consume both budgets at once.
	queues := map[string][]int{"follow": {}, "discover": {}}
	now := time.Now().UTC()
	for i, source := range candidates {
		if !bounds.Allows(source.URL) {
			if source.Kind == "public_candidate" {
				outsideDepth++
			}
			continue
		}
		for _, mode := range []string{"follow", "discover"} {
			if tg.Due(source, mode, now) {
				queues[mode] = append(queues[mode], i)
			}
		}
	}
	for _, mode := range []string{"follow", "discover"} {
		if c.mode != mode && c.mode != "all" {
			continue
		}
		queue := queues[mode]
		sort.SliceStable(queue, func(i, j int) bool {
			a, b := candidates[queue[i]], candidates[queue[j]]
			ta, tb := a.CheckedAt, b.CheckedAt
			if mode == "follow" {
				ta, tb = a.FollowedAt, b.FollowedAt
			}
			if ta == nil {
				return tb != nil
			}
			if tb == nil {
				return false
			}
			return ta.Before(*tb)
		})
		if len(queue) > c.maxChannels {
			queue = queue[:c.maxChannels]
		}
		for _, i := range queue {
			source := &candidates[i]
			checked++
			var e error
			success := false
			interval := c.discoveryInterval
			if mode == "follow" {
				interval = c.followInterval
				if source.StorageID != "" && source.StorageID != storageID {
					return fmt.Errorf("storage changed for %s; use a separate registry to backfill the new destination", source.URL)
				}
				source.StorageID = storageID
				result, followErr := tg.Follow(ctx, client, source, tg.FollowOptions{MaxPages: c.maxPages}, durableSink, checkpoint, wait)
				e = followErr
				success = e == nil && source.State == "web_readable"
				log.Printf("follow %s state=%s pages=%d stored=%d complete=%t cursor=%d before=%d", source.URL, source.State, result.Pages, result.Stored, result.Complete, source.LastMessageID, source.BeforeID)
			} else {
				if e = wait(ctx); e != nil {
					return e
				}
				var articles []tg.Article
				articles, e = tg.Probe(ctx, client, source, c.maxMessages)
				if e == nil {
					success = source.State == "web_readable"
					if success {
						interval = c.followInterval
						source.Tracked = true
					}
					for _, a := range articles {
						if sink != nil {
							if e = durableSink(ctx, a); e != nil {
								return e
							}
						} else {
							discoverLinks(a)
						}
					}
				}
				log.Printf("discover %s state=%s messages=%d %s", source.URL, source.State, source.Messages, source.Error)
			}
			if e != nil || !success {
				interval = c.retryInterval
				if source.State == "preview_unavailable" {
					interval = c.discoveryInterval
				}
			}
			tg.DeferCheck(source, success, interval, time.Now().UTC())
			if err = checkpoint(); err != nil {
				return err
			}
			if e != nil {
				var networkErr *url.Error
				if errors.As(e, &networkErr) || ctx.Err() != nil {
					return fmt.Errorf("request through Tor failed; progress saved: %w", e)
				}
				var pageErr *tg.PageError
				if errors.As(e, &pageErr) {
					log.Printf("%s %s: %v; retry scheduled for %s", mode, source.URL, pageErr, source.NextCheckAt.Format(time.RFC3339))
					continue
				}
				// Storage and checkpoint errors still stop the cycle.
				return e
			}
		}
	}
	return nil
}

func saveRegistry(path string, candidates []tg.Candidate) error { return atomicJSON(path, candidates) }
func atomicJSON(path string, value interface{}) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".telegram-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(body); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
