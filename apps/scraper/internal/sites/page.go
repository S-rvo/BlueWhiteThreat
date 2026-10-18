package sites

import (
	"fmt"
	"log"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/db"
	"github.com/S-rvo/BlueWhiteThreat/pkg/csamfilter"
)

// ScrapedPage est le contenu brut d'une page .onion récupérée par le scraper,
// avec sa source conservée pour vérification par un analyste CTI.
type ScrapedPage struct {
	URL       string    `bson:"url"`
	Source    string    `bson:"source"`
	ScrapedAt time.Time `bson:"scraped_at"`
	Content   string    `bson:"content"`
}

// SaveScrapedPage upsert une page scrapée dans MongoDB.article, par URL unique.
// Les pages indexant du CSAM sont rejetées avant toute écriture.
func SaveScrapedPage(page ScrapedPage) (int, error) {
	if csamfilter.IsBlockedURL(page.URL) || csamfilter.ContainsBlockedContent(page.Content) {
		log.Printf("Contenu bloqué (CSAM) détecté sur %s — page rejetée, non persistée", page.URL)
		return 0, fmt.Errorf("contenu bloqué: %s", page.URL)
	}
	return db.SaveAllEntries("BlueWhiteThreat", "article", []ScrapedPage{page}, []string{"URL"})
}
