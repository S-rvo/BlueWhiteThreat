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

// RunDarkwebScraper traite toutes les URLs découvertes par le crawler et pas
// encore scrapées : récupère leur contenu via Tor, puis les persiste en Mongo.
func RunDarkwebScraper() {
	ctx := context.Background()
	redisClient := db.ConnectRedis()
	defer redisClient.Close()

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

	urls, err := db.GetUnscrapedURLs(ctx, redisClient)
	if err != nil {
		log.Printf("Erreur lecture des URLs non-scrapées: %v", err)
		return
	}
	if len(urls) == 0 {
		log.Println("Aucune nouvelle URL à scraper")
		return
	}
	log.Printf("%d nouvelles URLs à scraper", len(urls))

	configs, err := LoadSiteConfigs(sitesConfigPath())
	if err != nil {
		log.Printf("Erreur chargement sites.json: %v", err)
		configs = map[string]SiteConfig{}
	}

	proxies := torProxies()
	rotator := NewTorProxyRotator(proxies)
	workersPerProxy := scraperWorkersPerProxy()

	urlCh := make(chan string)
	var wg sync.WaitGroup
	for _, proxy := range proxies {
		for i := 0; i < workersPerProxy; i++ {
			wg.Add(1)
			go func(homeProxy string) {
				defer wg.Done()
				browser := newPooledBrowser(homeProxy)
				defer browser.close()
				for targetURL := range urlCh {
					err, permanent := scrapeWithRetry(browser, targetURL, configs, rotator)
					switch {
					case err == nil:
						if markErr := db.MarkURLScraped(ctx, redisClient, targetURL); markErr != nil {
							log.Printf("Erreur marquage %s comme scrapée: %v", targetURL, markErr)
						}
					case permanent:
						log.Printf("Échec permanent scraping %s: %v", targetURL, err)
						if markErr := db.MarkURLScraped(ctx, redisClient, targetURL); markErr != nil {
							log.Printf("Erreur marquage %s comme scrapée: %v", targetURL, markErr)
						}
					default:
						log.Printf("Échec transitoire scraping %s après %d tentatives: %v", targetURL, maxScrapeAttempts, err)
						if markErr := db.MarkURLScrapeFailed(ctx, redisClient, targetURL); markErr != nil {
							log.Printf("Erreur marquage %s comme échec transitoire: %v", targetURL, markErr)
						}
					}
				}
			}(proxy)
		}
	}
	for _, targetURL := range urls {
		urlCh <- targetURL
	}
	close(urlCh)
	wg.Wait()
}

// scrapeWithRetry retente une URL en échec transitoire sur un proxy Tor
// différent (rotator.Next()) avant de l'abandonner, et reconstruit le
// navigateur du worker à chaque échec -- un process Chrome ayant échoué
// (timeout, circuit mort) n'est pas digne de confiance pour l'URL suivante.
// Un échec permanent (ErrPermanentScrapeFailure) n'est jamais retenté.
func scrapeWithRetry(browser *pooledBrowser, targetURL string, configs map[string]SiteConfig, rotator *TorProxyRotator) (err error, permanent bool) {
	for attempt := 1; attempt <= maxScrapeAttempts; attempt++ {
		err = scrapeOneGuarded(browser, targetURL, configs)
		if err == nil {
			return nil, false
		}
		if errors.Is(err, ErrPermanentScrapeFailure) {
			return err, true
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
func scrapeOneGuarded(browser *pooledBrowser, targetURL string, configs map[string]SiteConfig) error {
	fetchCtx, cancelTimeout := context.WithTimeout(browser.ctx, fetchTimeout)
	defer cancelTimeout()
	done := make(chan error, 1)
	go func() {
		done <- scrapeOne(fetchCtx, targetURL, configs)
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(hangGuard):
		cancelTimeout()
		return fmt.Errorf("scrapeOne bloqué au-delà de %s, contexte annulé", hangGuard)
	}
}

func scrapeOne(ctx context.Context, targetURL string, configs map[string]SiteConfig) error {
	parsed, err := url.Parse(targetURL)
	if err != nil {
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
	if _, err := SaveScrapedPage(page); err != nil {
		if errors.Is(err, ErrBlockedContent) {
			return fmt.Errorf("%w: %v", ErrPermanentScrapeFailure, err)
		}
		return fmt.Errorf("échec sauvegarde Mongo: %w", err)
	}
	return nil
}
