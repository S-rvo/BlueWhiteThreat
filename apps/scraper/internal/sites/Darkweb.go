package sites

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/db"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
)

func sitesConfigPath() string {
	if p := os.Getenv("SITES_CONFIG_PATH"); p != "" {
		return p
	}
	return "./data/sites.json"
}

func torProxies() []string {
	return []string{"tor1:9050", "tor2:9050", "tor3:9050"}
}

// fetchTimeout borne la durée d'un fetch (login + navigation) sur une URL :
// un circuit Tor mort ou un onion service qui ne répond jamais ne doit pas
// bloquer indéfiniment le tick courant.
const fetchTimeout = 90 * time.Second

// hangGuard borne la durée qu'un worker attend scrapeOne avant d'abandonner.
// Constaté le 2026-10-09→10 : un worker est resté bloqué ~8h30 dans
// scrapeOne malgré fetchTimeout, le cancel de contexte n'étant pas honoré
// de façon fiable par chromedp (le process Chrome sous-jacent peut rester
// figé sur un circuit Tor mort sans réagir à l'annulation). La garde ne tue
// que le contexte de l'appel courant ; c'est scrapeWithRetry qui reconstruit
// ensuite tout le navigateur du worker (voir pooledBrowser.rebuild) pour ne
// pas réutiliser un process potentiellement resté figé.
const hangGuard = fetchTimeout + 30*time.Second

// maxScrapeAttempts borne le nombre de tentatives par URL avant de la
// considérer en échec transitoire. Un deuxième essai change de proxy Tor
// (voir scrapeWithRetry) : un circuit mort au premier essai n'est pas
// forcément mort au second, contrairement à un contenu bloqué (CSAM) ou une
// URL invalide, qui échouent toujours de la même façon (voir
// ErrPermanentScrapeFailure).
const maxScrapeAttempts = 2

// ErrPermanentScrapeFailure marque un échec qu'aucune retentative ne peut
// résoudre (URL invalide, contenu CSAM bloqué) -- à distinguer d'un échec
// réseau/Tor transitoire, qui mérite une fenêtre de re-scrape bien plus
// courte (voir staleRescrapeFailedMaxAge) qu'un succès ou un échec permanent.
var ErrPermanentScrapeFailure = errors.New("échec permanent")

// Sans la puissance de calcul de Google, la fraîcheur du contenu scrapé ne
// peut venir que d'un re-scrape périodique borné : un sweep modeste plutôt
// qu'un balayage massif que 3 proxies Tor + chromedp ne peuvent absorber.
// staleRescrapeLimit est plus petit que l'équivalent côté crawler car chaque
// re-scrape coûte une instance Chrome complète, pas un simple fetch HTML.
const (
	staleRescrapeMaxAge = 7 * 24 * time.Hour
	staleRescrapeLimit  = 100

	// staleRescrapeFailedMaxAge règle la fenêtre de re-scrape des échecs
	// transitoires (Tor/timeout), suivis séparément dans urls_scrape_failed_at
	// (voir db.MarkURLScrapeFailed) -- bien plus courte que
	// staleRescrapeMaxAge : un circuit Tor mort au moment T a de bonnes
	// chances de fonctionner à nouveau quelques heures plus tard, et une URL
	// fraîchement découverte ne doit pas être écartée 7 jours sur un seul
	// coup de malchance.
	staleRescrapeFailedMaxAge = 2 * time.Hour
)

// defaultWorkersPerProxy fixe la concurrence par proxy Tor. Un seul worker
// par proxy laissait la capacité de 3 proxies largement sous-utilisée (une
// URL à la fois par proxy) alors qu'un SocksPort Tor sert normalement
// plusieurs flux/circuits concurrents sans souci. maxWorkersPerProxy borne
// l'emballement (chaque worker garde un process Chrome vivant en permanence).
const (
	defaultWorkersPerProxy = 2
	maxWorkersPerProxy     = 6
)

func scraperWorkersPerProxy() int {
	raw := os.Getenv("SCRAPER_WORKERS_PER_PROXY")
	if raw == "" {
		return defaultWorkersPerProxy
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return defaultWorkersPerProxy
	}
	if n > maxWorkersPerProxy {
		return maxWorkersPerProxy
	}
	return n
}

var errScrapeStorage = errors.New("scrape storage unavailable")
var errScrapeBrowser = errors.New("scrape browser unavailable")
var errScrapePolicy = errors.New("scrape policy denied")
var errScrapeHostBusy = errors.New("render host is temporarily reserved")

// RunDarkwebScraper supervises a continuous pool and independent queue maintenance.
func RunDarkwebScraper(ctx context.Context, client *redis.Client, mongoClient *mongo.Client) {
	configs, err := LoadSiteConfigs(sitesConfigPath())
	if err != nil {
		log.Printf("Erreur chargement sites.json: %v", err)
		configs = map[string]SiteConfig{}
	}
	proxies := torProxies()
	rotator := NewTorProxyRotator(proxies)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		maintainScrapeQueue(ctx, client)
	}()
	for _, proxy := range proxies {
		for range scraperWorkersPerProxy() {
			wg.Add(1)
			go func() {
				defer wg.Done()
				browser := newPooledBrowserContext(ctx, proxy)
				defer browser.close()
				runScrapeWorker(ctx, workerActions{
					claim: func(ctx context.Context) (db.ScrapeClaim, error) {
						return db.ClaimScrape(ctx, client)
					},
					finish: func(ctx context.Context, claim db.ScrapeClaim, outcome db.ScrapeOutcome) error {
						return db.FinishScrape(ctx, client, claim, outcome)
					},
					release: func(ctx context.Context, claim db.ScrapeClaim) error {
						return db.ReleaseScrape(ctx, client, claim)
					},
					scrape: func(target string) (error, bool) {
						return scrapeWithRetry(browser, target, configs, rotator, mongoClient)
					},
					resetBrowser: func() { browser.rebuild(proxy) },
				}, waitForWork)
			}()
		}
	}
	wg.Wait()
}

func maintainScrapeQueue(ctx context.Context, client *redis.Client) {
	// Due retries are promoted independently from compatibility indexing and
	// refresh jobs. Retain the existing refresh budget per 30 minutes.
	retry := time.NewTicker(5 * time.Second)
	index := time.NewTicker(time.Minute)
	refresh := time.NewTicker(30 * time.Minute)
	defer retry.Stop()
	defer index.Stop()
	defer refresh.Stop()

	run := func(label string, fn func(context.Context) error) {
		bounded, cancel := context.WithTimeout(ctx, 4*time.Second)
		defer cancel()
		if err := fn(bounded); err != nil && ctx.Err() == nil {
			log.Printf("Maintenance scraper (%s): %v", label, err)
		}
	}
	retryDue := func() {
		run("retries", func(ctx context.Context) error {
			_, err := db.PromoteDueScrapeRetries(ctx, client, 100)
			return err
		})
	}
	indexLegacy := func() {
		run("index", func(ctx context.Context) error { return db.IndexScrapePending(ctx, client, 8) })
	}
	refreshOld := func() {
		run("revisites", func(ctx context.Context) error {
			_, err := db.RequeueStaleScrapedURLs(ctx, client, staleRescrapeMaxAge, staleRescrapeLimit)
			return err
		})
		run("anciens échecs", func(ctx context.Context) error {
			_, err := db.RequeueLegacyScrapeFailures(ctx, client, staleRescrapeFailedMaxAge, staleRescrapeLimit)
			return err
		})
	}
	retryDue()
	indexLegacy()
	refreshOld()
	for {
		select {
		case <-ctx.Done():
			return
		case <-retry.C:
			retryDue()
		case <-index.C:
			indexLegacy()
		case <-refresh.C:
			refreshOld()
		}
	}
}

type workerActions struct {
	claim        func(context.Context) (db.ScrapeClaim, error)
	finish       func(context.Context, db.ScrapeClaim, db.ScrapeOutcome) error
	release      func(context.Context, db.ScrapeClaim) error
	scrape       func(string) (error, bool)
	resetBrowser func()
}

func waitForWork(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func runScrapeWorker(ctx context.Context, actions workerActions, wait func(context.Context, time.Duration) bool) {
	idleDelay := 500 * time.Millisecond
	var idleFor time.Duration
	var idledBrowser bool
	redisBackoff := 2 * time.Second
	infraBackoff := 5 * time.Second
	infraFailures := 0

	for ctx.Err() == nil {
		claim, err := actions.claim(ctx)
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("Erreur réservation scraper (pause %s): %v", redisBackoff, err)
			}
			if !wait(ctx, redisBackoff) {
				return
			}
			redisBackoff = min(redisBackoff*2, 30*time.Second)
			continue
		}
		redisBackoff = 2 * time.Second
		if claim.URL == "" {
			if !idledBrowser && idleFor >= 30*time.Second {
				actions.resetBrowser()
				idledBrowser = true
			}
			if !wait(ctx, idleDelay) {
				return
			}
			idleFor += idleDelay
			idleDelay = min(idleDelay*2, 3*time.Second)
			continue
		}
		idleDelay, idleFor, idledBrowser = 500*time.Millisecond, 0, false
		release := func() {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
			defer cancel()
			if err := actions.release(cleanup, claim); err != nil {
				log.Print("Réservation scraper laissée à son expiration")
			}
		}
		if ctx.Err() != nil {
			release()
			return
		}
		err, permanent := actions.scrape(claim.URL)
		if ctx.Err() != nil {
			release()
			return
		}
		outcome := db.ScrapeSuccess
		switch {
		case errors.Is(err, errScrapePolicy):
			outcome = db.ScrapeBlocked
		case permanent:
			outcome = db.ScrapePermanent
		case errors.Is(err, errScrapeHostBusy):
			outcome = db.ScrapeDeferred
		case errors.Is(err, errScrapeStorage), errors.Is(err, errScrapeBrowser):
			outcome = db.ScrapeInfrastructure
		case err != nil:
			outcome = db.ScrapeTransient
		}
		if err != nil {
			log.Printf("Scrape %s (%s): %v", claim.URL, outcome, err)
		}
		if err := actions.finish(ctx, claim, outcome); err != nil {
			log.Printf("Erreur confirmation scraping: %v", err)
			if ctx.Err() != nil {
				release()
				return
			}
			if !wait(ctx, redisBackoff) {
				return
			}
		}
		if outcome == db.ScrapeInfrastructure {
			infraFailures++
			if infraFailures >= 3 {
				actions.resetBrowser()
				log.Printf("Pause du worker scraper (infrastructure): %s", infraBackoff)
				if !wait(ctx, infraBackoff) {
					return
				}
				infraBackoff = min(infraBackoff*2, time.Minute)
			}
		} else {
			infraFailures, infraBackoff = 0, 5*time.Second
		}
	}
}

// scrapeWithRetry retente une URL en échec transitoire sur un proxy Tor
// différent (rotator.Next()) avant de l'abandonner, et reconstruit le
// navigateur du worker à chaque échec -- un process Chrome ayant échoué
// (timeout, circuit mort) n'est pas digne de confiance pour l'URL suivante.
// Un échec permanent (ErrPermanentScrapeFailure) n'est jamais retenté.
func scrapeWithRetry(browser *pooledBrowser, targetURL string, configs map[string]SiteConfig, rotator *TorProxyRotator, client *mongo.Client) (err error, permanent bool) {
	for attempt := 1; attempt <= maxScrapeAttempts; attempt++ {
		err = scrapeOneGuarded(browser, targetURL, configs, client)
		if err == nil {
			return nil, false
		}
		if errors.Is(err, ErrPermanentScrapeFailure) || errors.Is(err, errScrapePolicy) {
			return err, true
		}
		if errors.Is(err, errScrapeStorage) || errors.Is(err, errScrapeHostBusy) || browser.parent.Err() != nil {
			return err, false
		}
		if errors.Is(err, errScrapeBrowser) {
			browser.rebuild(browser.proxy)
			continue
		}
		retryProxy := rotator.Next()
		if attempt < maxScrapeAttempts {
			log.Printf("Échec transitoire scraping %s (tentative %d/%d), nouvel essai via %s: %v", targetURL, attempt, maxScrapeAttempts, retryProxy, err)
		}
		browser.rebuild(retryProxy)
	}
	return err, false
}

// scrapeOneGuarded lance scrapeOne dans une goroutine séparée et borne son
// attente, sans démonter le navigateur persistant du worker directement --
// c'est à l'appelant (scrapeWithRetry) de décider de le reconstruire.
func scrapeOneGuarded(browser *pooledBrowser, targetURL string, configs map[string]SiteConfig, clients ...*mongo.Client) error {
	var client *mongo.Client
	if len(clients) != 0 {
		client = clients[0]
	}
	tab, closeTab, err := browser.newTab()
	if err != nil {
		return fmt.Errorf("%w: %w", errScrapeBrowser, err)
	}
	defer closeTab()
	fetchCtx, cancelTimeout := context.WithTimeout(tab, fetchTimeout)
	defer cancelTimeout()
	done := make(chan error, 1)
	go func() {
		done <- scrapeOneWithClient(fetchCtx, targetURL, configs, client)
	}()
	select {
	case err := <-done:
		if errors.Is(err, context.Canceled) {
			return fmt.Errorf("%w: %w", errScrapeBrowser, err)
		}
		return err
	case <-browser.ctx.Done():
		return fmt.Errorf("%w: %w", errScrapeBrowser, browser.ctx.Err())
	case <-time.After(hangGuard):
		cancelTimeout()
		return fmt.Errorf("%w: tâche bloquée au-delà de %s", errScrapeBrowser, hangGuard)
	}
}

func scrapeOne(ctx context.Context, targetURL string, configs map[string]SiteConfig) error {
	return scrapeOneWithClient(ctx, targetURL, configs, nil)
}

func scrapeOneWithClient(ctx context.Context, targetURL string, configs map[string]SiteConfig, client *mongo.Client) error {
	parsed, err := url.Parse(targetURL)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%w: url invalide: %v", ErrPermanentScrapeFailure, err)
	}

	var cfg *SiteConfig
	if c, ok := configs[parsed.Hostname()]; ok {
		cfg = &c
	}

	content, err := FetchPage(ctx, targetURL, cfg)
	if err != nil {
		return err
	}

	page := ScrapedPage{
		URL:        targetURL,
		Source:     targetURL,
		SourceType: SourceTypeDarkwebPage,
		ScrapedAt:  time.Now().UTC(),
		Content:    content,
	}
	if _, err := saveScrapedPage(ctx, client, page); err != nil {
		if errors.Is(err, ErrBlockedContent) {
			return fmt.Errorf("%w: %v", errScrapePolicy, err)
		}
		return fmt.Errorf("%w: %w", errScrapeStorage, err)
	}
	return nil
}
