package db

import (
	"strings"
	"testing"

	"github.com/S-rvo/BlueWhiteThreat/pkg/contract"
	"github.com/S-rvo/BlueWhiteThreat/pkg/contract/contracttest"
)

// TestScraperKeysComeFromContract casse si le crawler écrit une clé Redis du
// scraper (Go ou Lua embarqué) qui n'est pas déclarée dans le contrat.
func TestScraperKeysComeFromContract(t *testing.T) {
	scraperOwned := func(key string) bool {
		return strings.HasPrefix(key, "scraper:") || strings.HasPrefix(key, "urls_scrape")
	}
	contracttest.RequireForeignKeysShared(t, "../..", scraperOwned, contract.SharedRedisKeys())
}

// TestCrawlerOwnedKeysFollowContract casse si le crawler renomme une clé que
// le scraper lit via pkg/contract (ou l'inverse).
func TestCrawlerOwnedKeysFollowContract(t *testing.T) {
	var owned []string
	for _, key := range contract.SharedRedisKeys() {
		switch key {
		case contract.KeyScraperPending, contract.KeyScraperInflight, contract.KeyURLsScraped,
			contract.KeyCrawlerRenderHostOwner: // écrite par le scraper seul
			continue
		}
		owned = append(owned, key)
	}
	contracttest.RequireKeysReferenced(t, "../..", owned)
}
