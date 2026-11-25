package db

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// ConnectRedis se connecte à l'instance Redis partagée avec le crawler.
func ConnectRedis() *redis.Client {
	host := os.Getenv("REDIS_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("REDIS_PORT")
	if port == "" {
		port = "6379"
	}
	return redis.NewClient(&redis.Options{
		Addr:     host + ":" + port,
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})
}

// GetUnscrapedURLs renvoie les URLs présentes dans "urls_crawled" (rempli par
// le crawler) mais absentes de "urls_scraped" (rempli par ce scraper).
func GetUnscrapedURLs(ctx context.Context, client *redis.Client) ([]string, error) {
	urls, err := client.SDiff(ctx, "urls_crawled", "urls_scraped").Result()
	if err != nil {
		return nil, err
	}
	limit, err := ScrapeBatchSize()
	if err != nil {
		return nil, err
	}
	freshness, err := crawlTimestamps(ctx, client, urls)
	if err != nil {
		return nil, err
	}
	return prioritizeURLs(urls, freshness, limit), nil
}

// crawlTimestamps récupère, en un seul aller-retour (ZMScore), l'horodatage de
// crawl de chaque URL candidate depuis "urls_crawled_at" (sorted set rempli par
// le crawler lorsqu'il verse une URL dans "urls_crawled"). Sert de clé de
// fraîcheur à prioritizeURLs. Une URL sans score -- invariant normalement
// garanti côté crawler, mais on ne s'y fie pas aveuglément -- est traitée comme
// la plus ancienne (0) : scrapée en dernier au sein de son tier CTI, jamais
// écartée.
func crawlTimestamps(ctx context.Context, client *redis.Client, urls []string) (map[string]float64, error) {
	if len(urls) == 0 {
		return nil, nil
	}
	scores, err := client.ZMScore(ctx, "urls_crawled_at", urls...).Result()
	if err != nil {
		return nil, err
	}
	fresh := make(map[string]float64, len(urls))
	for i, u := range urls {
		if i >= len(scores) {
			break
		}
		s := scores[i]
		if math.IsNaN(s) {
			s = 0
		}
		fresh[u] = s
	}
	return fresh, nil
}

// priorityScore only schedules collection; it never admits or rejects a URL.
// Strong CTI expressions are processed first. Ambiguous words need a second
// independent CTI signal, limiting the generic "access"/"database" noise that
// previously occupied entire scraper cycles. Hash-only URLs remain eligible.
func priorityScore(rawURL string) int {
	low := strings.ToLower(rawURL)
	strong := []string{
		"initial-access", "access-broker", "ransomware", "infostealer",
		"credential-dump", "data-breach", "zero-day", "0day", "botnet",
	}
	for _, term := range strong {
		if strings.Contains(low, term) {
			return 2
		}
	}
	ambiguous := []string{"access", "leak", "database", "dump", "credential"}
	corroborating := []string{"broker", "breach", "ransom", "malware", "stealer", "exploit", "victim", "corporate", "rdp", "vpn"}
	hasAmbiguous, hasCorroborating := false, false
	for _, term := range ambiguous {
		hasAmbiguous = hasAmbiguous || strings.Contains(low, term)
	}
	for _, term := range corroborating {
		hasCorroborating = hasCorroborating || strings.Contains(low, term)
	}
	if hasAmbiguous && hasCorroborating {
		return 1
	}
	return 0
}

// prioritizeURLs ordonne les candidates par pertinence CTI (priorityScore)
// d'abord, puis par fraîcheur de crawl décroissante (freshness[url], horodatage
// Unix issu de "urls_crawled_at") : à pertinence égale, on scrape d'abord ce qui
// vient d'être découvert, quand la chance de trouver le service .onion encore en
// vie est la plus haute. Le hash sha256 ne sert plus que de départage final
// déterministe (fraîcheur identique ou absente). La pertinence reste la clé
// primaire : une URL CTI forte mais ancienne passe toujours avant une URL
// fraîche sans signal.
func prioritizeURLs(urls []string, freshness map[string]float64, limit int) []string {
	type ranked struct {
		url      string
		priority int
		crawlTS  float64
		tie      string
	}
	rows := make([]ranked, len(urls))
	for i, url := range urls {
		rows[i] = ranked{
			url:      url,
			priority: priorityScore(url),
			crawlTS:  freshness[url],
			tie:      fmt.Sprintf("%x", sha256.Sum256([]byte(url))),
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].priority != rows[j].priority {
			return rows[i].priority > rows[j].priority
		}
		if rows[i].crawlTS != rows[j].crawlTS {
			return rows[i].crawlTS > rows[j].crawlTS
		}
		return rows[i].tie < rows[j].tie
	})
	if limit > len(rows) {
		limit = len(rows)
	}
	result := make([]string, limit)
	for i := range result {
		result[i] = rows[i].url
	}
	return result
}

// MarkURLScraped marque une URL comme traitée pour ne pas la retraiter au tick suivant.
func MarkURLScraped(ctx context.Context, client *redis.Client, url string) error {
	pipe := client.TxPipeline()
	pipe.SAdd(ctx, "urls_scraped", url)
	pipe.ZRem(ctx, "urls_scrape_failed_at", url)
	pipe.ZRem(ctx, "scraper:retry_due", url)
	pipe.ZAdd(ctx, "urls_scraped_at", redis.Z{Score: float64(time.Now().Unix()), Member: url})
	_, err := pipe.Exec(ctx)
	return err
}

// RequeueStaleScrapedURLs ré-expose à GetUnscrapedURLs les URLs scrapées il y
// a plus de maxAge, en les retirant de "urls_scraped"/"urls_scraped_at" --
// elles réapparaissent naturellement via le SDiff existant, sans file
// parallèle. Borné par limit : un sweep modeste, pas un balayage massif
// qu'on n'a pas la capacité Tor/chromedp d'absorber.
func RequeueStaleScrapedURLs(ctx context.Context, client *redis.Client, maxAge time.Duration, limit int64) (int64, error) {
	return requeueStaleFromSortedSet(ctx, client, "urls_scraped_at", maxAge, limit)
}

// MarkURLScrapeFailed marque une URL comme en échec transitoire (réseau/Tor,
// pas un échec permanent -- voir sites.ErrPermanentScrapeFailure) dans un
// sorted set séparé de "urls_scraped_at", avec sa propre fenêtre de
// re-exposition bien plus courte (voir RequeueFailedScrapedURLs) : un échec
// transitoire ne doit pas être traité comme un succès ni hériter de son délai.
func MarkURLScrapeFailed(ctx context.Context, client *redis.Client, url string) error {
	pipe := client.TxPipeline()
	pipe.SAdd(ctx, "urls_scraped", url)
	pipe.ZRem(ctx, "urls_scraped_at", url)
	pipe.ZRem(ctx, "scraper:retry_due", url)
	pipe.ZAdd(ctx, "urls_scrape_failed_at", redis.Z{Score: float64(time.Now().Unix()), Member: url})
	_, err := pipe.Exec(ctx)
	return err
}

// RequeueFailedScrapedURLs ré-expose à GetUnscrapedURLs les URLs en échec
// transitoire depuis plus de maxAge, selon le même mécanisme que
// RequeueStaleScrapedURLs mais sur le sorted set "urls_scrape_failed_at".
func RequeueFailedScrapedURLs(ctx context.Context, client *redis.Client, maxAge time.Duration, limit int64) (int64, error) {
	n, err := requeueStaleFromSortedSet(ctx, client, "scraper:retry_due", 0, limit)
	if err != nil || n >= limit {
		return n, err
	}
	legacy, err := requeueStaleFromSortedSet(ctx, client, "urls_scrape_failed_at", maxAge, limit-n)
	return n + legacy, err
}

func requeueStaleFromSortedSet(ctx context.Context, client *redis.Client, sortedSetKey string, maxAge time.Duration, limit int64) (int64, error) {
	cutoff := time.Now().Add(-maxAge).Unix()
	if limit < 1 {
		return 0, nil
	}
	// The legacy failure index may also include backoff-controlled URLs. Scan
	// past those entries in bounded pages instead of stopping at the first page.
	offset := int64(0)
	if sortedSetKey == "urls_scrape_failed_at" {
		cursor, err := client.Get(ctx, "scraper:legacy_failed_offset").Int64()
		if err != nil && err != redis.Nil {
			return 0, err
		}
		offset = cursor
	}
	stale, err := client.ZRangeByScore(ctx, sortedSetKey, &redis.ZRangeBy{
		Min: "-inf", Max: fmt.Sprint(cutoff), Offset: offset, Count: limit,
	}).Result()
	if err != nil {
		return 0, err
	}
	args := make([]interface{}, 0, 3+2*len(stale))
	args = append(args, cutoff, offset, limit)
	for _, u := range stale {
		args = append(args, u, priorityScore(u)*priorityStride)
	}
	return requeueScrapeScript.Run(ctx, client, []string{sortedSetKey, scrapePending}, args...).Int64()
}

var requeueScrapeScript = redis.NewScript(`
local n=0
for i=4,#ARGV,2 do
 local u=ARGV[i]
 local score=redis.call('ZSCORE',KEYS[1],u)
 if score and tonumber(score)<=tonumber(ARGV[1])
    and (KEYS[1]~='urls_scrape_failed_at' or not redis.call('ZSCORE','scraper:retry_due',u))
    and not redis.call('ZSCORE','scraper:inflight',u) then
  redis.call('ZREM',KEYS[1],u)
  if KEYS[1]=='scraper:retry_due' then redis.call('ZREM','urls_scrape_failed_at',u) end
  -- If a legacy URL has both marks, only the newer outcome may reopen it.
  if not redis.call('ZSCORE','urls_scraped_at',u)
     and not redis.call('ZSCORE','urls_scrape_failed_at',u)
     and not redis.call('ZSCORE','scraper:dead',u)
     and not redis.call('ZSCORE','scraper:invalid',u)
     and not redis.call('ZSCORE','scraper:blocked',u) then
   redis.call('SREM','urls_scraped',u)
   if redis.call('SISMEMBER','urls_crawled',u)==1 then
    local ts=tonumber(redis.call('ZSCORE','urls_crawled_at',u) or '0')
    redis.call('ZADD',KEYS[2],tonumber(ARGV[i+1])+ts,u)
   end
  end
  n=n+1
 end
end
if KEYS[1]=='urls_scrape_failed_at' then
 local scanned=(#ARGV-3)/2
 local offset=tonumber(ARGV[2])+scanned-n
 if scanned<tonumber(ARGV[3]) then offset=0 end
 redis.call('SET','scraper:legacy_failed_offset',offset)
end
return n
`)
