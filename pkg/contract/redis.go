package contract

import "strings"

// Clés Redis lues ou écrites par plus d'un service. Les clés internes à un
// seul service restent locales à ce service. Dans les scripts Lua, ces noms
// apparaissent en littéral : les tests de divergence de chaque service
// vérifient qu'aucun littéral d'un espace partagé n'échappe à cette liste.
const (
	// Frontière du crawler, lue par le scraper (statut, admission).
	KeyURLsToCrawl      = "urls_to_crawl"
	KeyURLsQueued       = "urls_queued" // champ de crawler:stats
	KeyURLsCrawled      = "urls_crawled"
	KeyURLsCrawledAt    = "urls_crawled_at"
	KeyCrawlerScheduled = "crawler:scheduled"
	KeyCrawlerInflight  = "crawler:inflight"
	KeyCrawlerStats     = "crawler:stats"
	KeyCrawlerGone      = "crawler:gone"
	KeyCrawlerBlocked   = "crawler:blocked"

	// Politesse par hôte : le scraper rendu réserve un créneau sur l'hôte.
	KeyCrawlerHosts               = "crawler:hosts"
	KeyCrawlerHostReady           = "crawler:host_ready"
	KeyCrawlerScheduledHosts      = "crawler:scheduled_hosts"
	KeyCrawlerScheduledHostPrefix = "crawler:scheduled_host:"
	KeyCrawlerRenderedLinks       = "crawler:rendered_links"
	KeyCrawlerRenderedInflight    = "crawler:rendered_inflight"
	KeyCrawlerRenderedRetry       = "crawler:rendered_retry"
	// Propriétaire d'un hôte en cours de rendu : écrit par le scraper seul,
	// mais dans l'espace "crawler:" — réservé ici pour que le crawler ne le
	// réutilise pas à un autre usage.
	KeyCrawlerRenderHostOwner = "crawler:render_host_owner"

	// File du scraper, alimentée directement par le crawler à chaque succès.
	KeyScraperPending  = "scraper:pending"
	KeyScraperInflight = "scraper:inflight"
	KeyURLsScraped     = "urls_scraped"
)

// SharedRedisKeys renvoie toutes les clés (ou préfixes) du contrat Redis.
func SharedRedisKeys() []string {
	return []string{
		KeyURLsToCrawl, KeyURLsQueued, KeyURLsCrawled, KeyURLsCrawledAt,
		KeyCrawlerScheduled, KeyCrawlerInflight, KeyCrawlerStats, KeyCrawlerGone, KeyCrawlerBlocked,
		KeyCrawlerHosts, KeyCrawlerHostReady, KeyCrawlerScheduledHosts, KeyCrawlerScheduledHostPrefix,
		KeyCrawlerRenderedLinks, KeyCrawlerRenderedInflight, KeyCrawlerRenderedRetry, KeyCrawlerRenderHostOwner,
		KeyScraperPending, KeyScraperInflight, KeyURLsScraped,
	}
}

// PriorityStride sépare les niveaux de priorité dans le score de
// scraper:pending : score = URLPriority*PriorityStride + horodatage Unix. Il
// doit dépasser tout horodatage pour qu'un niveau supérieur passe toujours
// devant, quelle que soit la fraîcheur.
const PriorityStride int64 = 10_000_000_000

// URLPriority ordonne la collecte ; il n'admet ni ne rejette aucune URL. Les
// expressions CTI fortes passent en premier. Un terme ambigu doit être
// corroboré par un second signal indépendant, pour limiter le bruit des
// "access"/"database" génériques. Les URL sans signal restent éligibles (0).
func URLPriority(rawURL string) int {
	low := strings.ToLower(rawURL)
	for _, term := range strongPriorityTerms {
		if strings.Contains(low, term) {
			return 2
		}
	}
	hasAmbiguous, hasCorroborating := false, false
	for _, term := range ambiguousPriorityTerms {
		hasAmbiguous = hasAmbiguous || strings.Contains(low, term)
	}
	for _, term := range corroboratingPriorityTerms {
		hasCorroborating = hasCorroborating || strings.Contains(low, term)
	}
	if hasAmbiguous && hasCorroborating {
		return 1
	}
	return 0
}

// PendingPriorityBase renvoie la part priorité du score scraper:pending, à
// laquelle chaque service ajoute l'horodatage Unix pertinent.
func PendingPriorityBase(rawURL string) int64 {
	return int64(URLPriority(rawURL)) * PriorityStride
}

var (
	strongPriorityTerms = []string{
		"initial-access", "access-broker", "ransomware", "infostealer",
		"credential-dump", "data-breach", "zero-day", "0day", "botnet",
	}
	ambiguousPriorityTerms     = []string{"access", "leak", "database", "dump", "credential"}
	corroboratingPriorityTerms = []string{"broker", "breach", "ransom", "malware", "stealer", "exploit", "victim", "corporate", "rdp", "vpn"}
)
