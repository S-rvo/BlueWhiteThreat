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
	"sync/atomic"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/db"
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

// RunDarkwebScraper processes a bounded cycle from the shared Redis frontier.
func RunDarkwebScraper(ctx context.Context, mongoClient *mongo.Client) {
	redisClient := db.ConnectRedis()
	defer redisClient.Close()
	maxBatch, err := db.ScrapeBatchSize()
	if err != nil {
		log.Print(err)
		return
	}
	if requeued, err := db.RequeueStaleScrapedURLs(ctx, redisClient, staleRescrapeMaxAge, staleRescrapeLimit); err != nil {
		log.Printf("Erreur requeue des URLs scrapées obsolètes: %v", err)
	} else if requeued > 0 {
		log.Printf("%d URLs scrapées obsolètes remises en file pour re-scrape", requeued)
	}
	if requeued, err := db.RequeueFailedScrapedURLs(ctx, redisClient, staleRescrapeFailedMaxAge, staleRescrapeLimit); err != nil {
		log.Printf("Erreur requeue des URLs en échec transitoire: %v", err)
	} else if requeued > 0 {
		log.Printf("%d URLs en échec transitoire remises en file", requeued)
	}
	if err := db.IndexScrapePending(ctx, redisClient, 8); err != nil {
		log.Printf("Erreur indexation de la file scraper: %v", err)
		return
	}

	configs, err := LoadSiteConfigs(sitesConfigPath())
	if err != nil {
		log.Printf("Erreur chargement sites.json: %v", err)
		configs = map[string]SiteConfig{}
	}
	proxies := torProxies()
	rotator := NewTorProxyRotator(proxies)
	workersPerProxy := scraperWorkersPerProxy()
	var slots, completed atomic.Int64
	var wg sync.WaitGroup
	for _, proxy := range proxies {
		for i := 0; i < workersPerProxy; i++ {
			wg.Add(1)
			go func(homeProxy string) {
				defer wg.Done()
				browser := newPooledBrowserContext(ctx, homeProxy)
				defer browser.close()
				consecutiveInfra := 0
				for ctx.Err() == nil && slots.Add(1) <= int64(maxBatch) {
					claim, err := db.ClaimScrape(ctx, redisClient)
					if err != nil {
						log.Printf("Erreur réservation scraper: %v", err)
						return
					}
					if claim.URL == "" {
						return
					}
					err, permanent := scrapeWithRetry(browser, claim.URL, configs, rotator, mongoClient)
					if ctx.Err() != nil {
						return // A lease is recovered automatically after expiry.
					}
					outcome := db.ScrapeSuccess
					switch {
					case errors.Is(err, errScrapePolicy):
						outcome = db.ScrapeBlocked
					case permanent:
						outcome = db.ScrapePermanent
					case errors.Is(err, errScrapeStorage), errors.Is(err, errScrapeBrowser):
						outcome = db.ScrapeInfrastructure
					case err != nil:
						outcome = db.ScrapeTransient
					}
					if err != nil {
						log.Printf("Scrape %s (%s): %v", claim.URL, outcome, err)
					}
					if markErr := db.FinishScrape(ctx, redisClient, claim, outcome); markErr != nil {
						log.Printf("Erreur confirmation scraping: %v", markErr)
					} else {
						completed.Add(1)
					}
					if outcome == db.ScrapeInfrastructure {
						consecutiveInfra++
						if consecutiveInfra >= 3 {
							log.Print("Pause du worker scraper après trois erreurs d'infrastructure")
							return
						}
					} else {
						consecutiveInfra = 0
					}
				}
			}(proxy)
		}
	}
	wg.Wait()
	log.Printf("Cycle scraper: %d résultats confirmés", completed.Load())
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
		if errors.Is(err, errScrapeStorage) || browser.parent.Err() != nil {
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
