package sites

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
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

	rotator := NewTorProxyRotator(torProxies())

	for _, targetURL := range urls {
		if err := scrapeOne(ctx, targetURL, configs, rotator); err != nil {
			log.Printf("Erreur scraping %s: %v", targetURL, err)
			continue
		}
		if err := db.MarkURLScraped(ctx, redisClient, targetURL); err != nil {
			log.Printf("Erreur marquage %s comme scrapée: %v", targetURL, err)
		}
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

	allocCtx, cancelAlloc := newChromedpContext(ctx, rotator.Next())
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
