package sites

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
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
// figé sur un circuit Tor mort sans réagir à l'annulation). Sans cette
// garde, wg.Wait() ne revient jamais et tous les ticks suivants sont gelés.
const hangGuard = fetchTimeout + 30*time.Second

// RunDarkwebScraper traite toutes les URLs découvertes par le crawler et pas
// encore scrapées : récupère leur contenu via Tor, puis les persiste en Mongo.
func RunDarkwebScraper() {
	ctx := context.Background()
	redisClient := db.ConnectRedis()
	defer redisClient.Close()

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

	// Un worker par proxy Tor : avant ce changement une seule URL était
	// traitée à la fois malgré 3 proxies disponibles (2 restaient inutilisés
	// en permanence). Le rotator distribue déjà les proxies en round-robin
	// thread-safe, donc il suffit de paralléliser les appels à scrapeOne.
	urlCh := make(chan string)
	var wg sync.WaitGroup
	for i := 0; i < len(proxies); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for targetURL := range urlCh {
				if err := scrapeOneGuarded(ctx, targetURL, configs, rotator); err != nil {
					log.Printf("Erreur scraping %s: %v", targetURL, err)
				}
				// Marquée comme traitée même en cas d'échec : sinon une URL morte ou
				// bloquée (CSAM) est retentée indéfiniment à chaque cycle, polluant
				// la queue pour toujours (constaté le 2026-10-09 : mêmes URLs mortes
				// retentées 10+ fois sans jamais sortir de la queue).
				if err := db.MarkURLScraped(ctx, redisClient, targetURL); err != nil {
					log.Printf("Erreur marquage %s comme scrapée: %v", targetURL, err)
				}
			}
		}()
	}
	for _, targetURL := range urls {
		urlCh <- targetURL
	}
	close(urlCh)
	wg.Wait()
}

// scrapeOneGuarded lance scrapeOne dans une goroutine séparée et abandonne
// l'attente au bout de hangGuard. La goroutine scrapeOne peut fuiter si
// chromedp ignore vraiment le cancel, mais le worker, lui, repart
// immédiatement traiter l'URL suivante : c'est le pire cas (fuite mémoire
// lente) qu'on accepte plutôt que le blocage total déjà constaté.
func scrapeOneGuarded(ctx context.Context, targetURL string, configs map[string]SiteConfig, rotator *TorProxyRotator) error {
	done := make(chan error, 1)
	go func() {
		done <- scrapeOne(ctx, targetURL, configs, rotator)
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(hangGuard):
		return fmt.Errorf("scrapeOne bloqué au-delà de %s, worker libéré (goroutine abandonnée)", hangGuard)
	}
}

func scrapeOne(ctx context.Context, targetURL string, configs map[string]SiteConfig, rotator *TorProxyRotator) error {
	parsed, err := url.Parse(targetURL)
	if err != nil {
		return fmt.Errorf("url invalide: %w", err)
	}

	var cfg *SiteConfig
	if c, ok := configs[parsed.Hostname()]; ok {
		cfg = &c
	}

	fetchCtx, cancelTimeout := context.WithTimeout(ctx, fetchTimeout)
	defer cancelTimeout()

	allocCtx, cancelAlloc := newChromedpContext(fetchCtx, rotator.Next())
	defer cancelAlloc()

	content, err := FetchPage(allocCtx, targetURL, cfg)
	if err != nil {
		return err
	}

	page := ScrapedPage{
		URL:       targetURL,
		Source:    targetURL,
		ScrapedAt: time.Now().UTC(),
		Content:   content,
	}
	if _, err := SaveScrapedPage(page); err != nil {
		return fmt.Errorf("échec sauvegarde Mongo: %w", err)
	}
	return nil
}
