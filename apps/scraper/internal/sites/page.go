package sites

import (
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/db"
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
func SaveScrapedPage(page ScrapedPage) (int, error) {
	return db.SaveAllEntries("BlueWhiteThreat", "article", []ScrapedPage{page}, []string{"URL"})
}
