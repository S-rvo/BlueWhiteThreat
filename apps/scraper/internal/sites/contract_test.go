package sites

import (
	"strings"
	"testing"

	"github.com/S-rvo/BlueWhiteThreat/pkg/contract"
	"github.com/S-rvo/BlueWhiteThreat/pkg/contract/contracttest"
)

// TestScrapedPageMatchesContract casse si le document écrit par le scraper
// s'écarte du schéma article lu par le classifier, le parser et l'api.
func TestScrapedPageMatchesContract(t *testing.T) {
	contracttest.RequireBSONTagsEqual(t, ScrapedPage{}, []string{
		contract.FieldURL, contract.FieldSource, contract.FieldSourceType,
		contract.FieldScrapedAt, contract.FieldContent,
	})
	contracttest.RequireBSONTagsIn(t, ScrapedPage{}, contract.ArticleFields())
}

// TestCrawlerKeysComeFromContract casse si le scraper lit ou écrit une clé
// Redis du crawler (Go ou Lua embarqué) qui n'est pas déclarée dans le contrat.
func TestCrawlerKeysComeFromContract(t *testing.T) {
	crawlerOwned := func(key string) bool {
		return strings.HasPrefix(key, "crawler:") || strings.HasPrefix(key, "urls_crawl") ||
			key == contract.KeyURLsToCrawl || key == contract.KeyURLsQueued
	}
	contracttest.RequireForeignKeysShared(t, "../..", crawlerOwned, contract.SharedRedisKeys())
}

// TestScraperOwnedKeysFollowContract casse si le contrat renomme une clé du
// scraper que ses scripts Lua écrivent encore sous l'ancien nom.
func TestScraperOwnedKeysFollowContract(t *testing.T) {
	contracttest.RequireKeysReferenced(t, "../..", []string{
		contract.KeyScraperInflight, contract.KeyURLsScraped, contract.KeyCrawlerRenderHostOwner,
	})
}
