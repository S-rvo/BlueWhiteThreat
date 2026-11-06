package db

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/pkg/csamfilter"
	"github.com/redis/go-redis/v9"
)

var (
	RedisClient *redis.Client
	ctx         = context.Background()
)

// InitRedis initialise la connexion à Redis
func InitRedis() error {
	redisHost := os.Getenv("REDIS_HOST")
	if redisHost == "" {
		redisHost = "localhost"
	}

	redisPort := os.Getenv("REDIS_PORT")
	if redisPort == "" {
		redisPort = "6379"
	}

	RedisClient = redis.NewClient(&redis.Options{
		Addr:     redisHost + ":" + redisPort,
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})

	_, err := RedisClient.Ping(ctx).Result()
	if err != nil {
		return err
	}

	log.Printf("Connected to Redis: %s:%s", redisHost, redisPort) // #nosec G706 -- values come from operator-set env vars, not user input
	return nil
}

// SetupRedisQueues initialise les structures de données nécessaires dans Redis
func SetupRedisQueues() error {
	keys := []string{"urls_to_crawl", "urls_crawled"}

	for _, key := range keys {
		exists, err := RedisClient.Exists(ctx, key).Result()
		if err != nil {
			return err
		}
		if exists == 0 {
			log.Printf("Initializing %s set in Redis", key)
		}
	}

	if exists, err := RedisClient.Exists(ctx, "crawler:stats").Result(); err != nil {
		return err
	} else if exists == 0 {
		log.Println("Initializing crawler:stats hash in Redis")
		RedisClient.HSet(ctx, "crawler:stats", map[string]interface{}{
			"urls_crawled": 0,
			"urls_queued":  0,
			"start_time":   time.Now().Unix(),
		})
	}

	// Rebuild the small scheduling index from the durable queue. This makes
	// prioritisation effective immediately for queues created by older builds;
	// urls_to_crawl remains the source of truth and no URL is discarded.
	if err := rebuildPriorityQueue(); err != nil {
		return fmt.Errorf("rebuild crawler priority index: %w", err)
	}

	// Backfill urls_crawled_at for members crawled before freshness tracking
	// existed. Seeded at "now" rather than their true (unknown) crawl time so
	// they don't all become immediately eligible for the staleness sweep at
	// once -- a burst we can't afford without Google's compute budget.
	if err := backfillCrawledAt(); err != nil {
		return fmt.Errorf("backfill urls_crawled_at: %w", err)
	}

	return nil
}

func backfillCrawledAt() error {
	now := float64(time.Now().Unix())
	var cursor uint64
	for {
		urls, next, err := RedisClient.SScan(ctx, "urls_crawled", cursor, "*", 1000).Result()
		if err != nil {
			return err
		}
		if len(urls) > 0 {
			members := make([]redis.Z, 0, len(urls))
			for _, url := range urls {
				members = append(members, redis.Z{Score: now, Member: url})
			}
			if err := RedisClient.ZAddArgs(ctx, "urls_crawled_at", redis.ZAddArgs{
				NX:      true,
				Members: members,
			}).Err(); err != nil {
				return err
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return nil
}

func rebuildPriorityQueue() error {
	if err := RedisClient.Del(ctx, "urls_to_crawl_priority").Err(); err != nil {
		return err
	}
	var cursor uint64
	for {
		urls, next, err := RedisClient.SScan(ctx, "urls_to_crawl", cursor, "*", 1000).Result()
		if err != nil {
			return err
		}
		priority := make([]interface{}, 0)
		for _, url := range urls {
			if queuePriority(url) > 0 {
				priority = append(priority, url)
			}
		}
		if len(priority) > 0 {
			if err := RedisClient.SAdd(ctx, "urls_to_crawl_priority", priority...).Err(); err != nil {
				return err
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return nil
}

// maxQueueSize borne la file d'attente : sans la puissance de calcul de
// Google, un seul worker à ~2 URLs/s ne peut pas absorber une file qui
// grossirait sans limite (seeds + liens découverts + requeue de fraîcheur).
// Au-delà, les nouveaux ajouts sont silencieusement ignorés (backpressure)
// plutôt que de laisser la file dériver vers une taille ingérable.
const maxQueueSize = 5000

func queueAtCapacity() (bool, error) {
	size, err := GetQueueSize()
	if err != nil {
		return false, err
	}
	return size >= maxQueueSize, nil
}

// QueueCapacityRatio expose maxQueueSize (privé) sous forme de ratio, pour
// que des outils externes (ex: cmd/alerting) puissent détecter une file qui
// approche la limite sans dupliquer la constante.
func QueueCapacityRatio() (float64, error) {
	size, err := GetQueueSize()
	if err != nil {
		return 0, err
	}
	return float64(size) / float64(maxQueueSize), nil
}

// AddURLToQueue ajoute une URL à la file d'attente si elle n'a pas déjà été traitée
func AddURLToQueue(url string) error {
	cleaned := cleanOnionURL(url)

	if csamfilter.IsBlockedURL(cleaned) || IsFarmBackend(cleaned) {
		return nil
	}

	atCapacity, err := queueAtCapacity()
	if err != nil {
		return err
	}
	if atCapacity {
		return nil
	}

	pipe := RedisClient.Pipeline()
	isCrawledCmd := pipe.SIsMember(ctx, "urls_crawled", cleaned)
	isQueuedCmd := pipe.SIsMember(ctx, "urls_to_crawl", cleaned)
	_, err = pipe.Exec(ctx)
	if err != nil {
		return err
	}

	isCrawled, _ := isCrawledCmd.Result()
	isQueued, _ := isQueuedCmd.Result()

	if !isCrawled && !isQueued {
		pipe := RedisClient.Pipeline()
		pipe.SAdd(ctx, "urls_to_crawl", cleaned)
		if queuePriority(cleaned) > 0 {
			pipe.SAdd(ctx, "urls_to_crawl_priority", cleaned)
		}
		if _, err = pipe.Exec(ctx); err != nil {
			return err
		}
		RedisClient.HIncrBy(ctx, "crawler:stats", "urls_queued", 1)
		log.Printf("Added URL to queue: %s", cleaned)
	}

	return nil
}

// GetNextURLFromQueue récupère une URL de la file d'attente et la supprime
func GetNextURLFromQueue() (string, error) {
	// Prefer the derived index first. Stale members are harmless and removed
	// while looking for one that is still present in the durable queue.
	for {
		priorityURL, err := RedisClient.SPop(ctx, "urls_to_crawl_priority").Result()
		if err == redis.Nil {
			break
		}
		if err != nil {
			return "", err
		}
		removed, err := RedisClient.SRem(ctx, "urls_to_crawl", priorityURL).Result()
		if err != nil {
			return "", err
		}
		if removed > 0 {
			return priorityURL, nil
		}
	}

	// Redis sets are unordered. Sampling several candidates preserves broad
	// collection while avoiding months of random backlog before descriptive
	// CTI URLs are visited. Existing queue entries benefit without migration.
	candidates, err := RedisClient.SRandMemberN(ctx, "urls_to_crawl", 512).Result()
	if err != nil {
		if err == redis.Nil {
			return "", nil
		}
		return "", err
	}
	if len(candidates) == 0 {
		return "", nil
	}
	url := selectPriorityURL(candidates)
	removed, err := RedisClient.SRem(ctx, "urls_to_crawl", url).Result()
	if err != nil {
		return "", err
	}
	if removed == 0 {
		return "", nil
	}
	return url, nil
}

func queuePriority(rawURL string) int {
	low := strings.ToLower(rawURL)
	for _, term := range []string{"initial-access", "access-broker", "ransomware", "infostealer", "credential-dump", "data-breach", "zero-day", "0day", "botnet"} {
		if strings.Contains(low, term) {
			return 2
		}
	}
	hasAmbiguous, hasCorroborating := false, false
	for _, term := range []string{"access", "leak", "database", "dump", "credential"} {
		hasAmbiguous = hasAmbiguous || strings.Contains(low, term)
	}
	for _, term := range []string{"broker", "breach", "ransom", "malware", "stealer", "exploit", "victim", "corporate", "rdp", "vpn"} {
		hasCorroborating = hasCorroborating || strings.Contains(low, term)
	}
	if hasAmbiguous && hasCorroborating {
		return 1
	}
	return 0
}

func selectPriorityURL(urls []string) string {
	best, bestPriority, bestTie := "", -1, ""
	for _, url := range urls {
		priority := queuePriority(url)
		tie := fmt.Sprintf("%x", sha256.Sum256([]byte(url)))
		if priority > bestPriority || (priority == bestPriority && (best == "" || tie < bestTie)) {
			best, bestPriority, bestTie = url, priority, tie
		}
	}
	return best
}

// MarkURLAsCrawled marque une URL comme visitée
func MarkURLAsCrawled(url string) error {
	cleaned := cleanOnionURL(url)
	pipe := RedisClient.Pipeline()
	pipe.SAdd(ctx, "urls_crawled", cleaned)
	pipe.ZAdd(ctx, "urls_crawled_at", redis.Z{Score: float64(time.Now().Unix()), Member: cleaned})
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}
	RedisClient.HIncrBy(ctx, "crawler:stats", "urls_crawled", 1)
	return nil
}

// RequeueStaleURLs re-surfaces already-crawled URLs whose last visit is older
// than maxAge, so the frontier doesn't go permanently stale once the backlog
// of brand-new URLs runs out. Bounded by limit: without Google's crawl
// budget, a sweep must stay small per call rather than requeuing everything
// stale at once. High-CTI-signal URLs are requeued first.
func RequeueStaleURLs(maxAge time.Duration, limit int64) (int64, error) {
	// Une file déjà pleine ne doit pas grossir davantage : le requeue de
	// fraîcheur cède la place aux nouveaux liens découverts plutôt que
	// d'aggraver un engorgement déjà en cours.
	atCapacity, err := queueAtCapacity()
	if err != nil {
		return 0, err
	}
	if atCapacity {
		return 0, nil
	}

	cutoff := time.Now().Add(-maxAge).Unix()
	stale, err := RedisClient.ZRangeByScore(ctx, "urls_crawled_at", &redis.ZRangeBy{
		Min:   "-inf",
		Max:   fmt.Sprintf("%d", cutoff),
		Count: limit,
	}).Result()
	if err != nil {
		return 0, err
	}
	if len(stale) == 0 {
		return 0, nil
	}

	sortByPriorityDesc(stale)

	now := float64(time.Now().Unix())
	var requeued int64
	for _, url := range stale {
		pipe := RedisClient.Pipeline()
		addedCmd := pipe.SAdd(ctx, "urls_to_crawl", url)
		// Bump the timestamp now, not after the crawl succeeds, so a stale
		// entry can't be pulled into every sweep until it's actually revisited.
		pipe.ZAdd(ctx, "urls_crawled_at", redis.Z{Score: now, Member: url})
		if _, err := pipe.Exec(ctx); err != nil {
			return requeued, err
		}
		if queuePriority(url) > 0 {
			if err := RedisClient.SAdd(ctx, "urls_to_crawl_priority", url).Err(); err != nil {
				return requeued, err
			}
		}
		if added, _ := addedCmd.Result(); added > 0 {
			requeued++
		}
	}
	if requeued > 0 {
		RedisClient.HIncrBy(ctx, "crawler:stats", "stale_requeued_total", requeued)
	}
	return requeued, nil
}

func sortByPriorityDesc(urls []string) {
	sort.Slice(urls, func(i, j int) bool {
		pi, pj := queuePriority(urls[i]), queuePriority(urls[j])
		if pi != pj {
			return pi > pj
		}
		return urls[i] < urls[j]
	})
}

// GetQueueSize renvoie le nombre d'URLs dans la file d'attente
func GetQueueSize() (int64, error) {
	return RedisClient.SCard(ctx, "urls_to_crawl").Result()
}

// GetCrawledCount renvoie le nombre d'URLs crawlées
func GetCrawledCount() (int64, error) {
	return RedisClient.SCard(ctx, "urls_crawled").Result()
}

// FetchCrawlerStats récupère les statistiques du crawler depuis Redis et les
// complète avec quelques métriques calculées à la volée (jamais stockées,
// donc jamais périmées) : taille de la file, nombre d'URLs crawlées, et âge
// de l'entrée la plus ancienne de urls_crawled_at -- un proxy bon marché pour
// détecter une dérive silencieuse de la fraîcheur sans calculer une médiane
// sur l'ensemble du ZSET (coût qu'on ne peut pas s'offrir à cette échelle).
func FetchCrawlerStats() (map[string]string, error) {
	stats, err := RedisClient.HGetAll(ctx, "crawler:stats").Result()
	if err != nil {
		return nil, err
	}
	if stats == nil {
		stats = map[string]string{}
	}

	if queueSize, err := GetQueueSize(); err == nil {
		stats["queue_size"] = fmt.Sprintf("%d", queueSize)
	}
	if crawledCount, err := GetCrawledCount(); err == nil {
		stats["crawled_count"] = fmt.Sprintf("%d", crawledCount)
	}
	if oldest, err := RedisClient.ZRangeWithScores(ctx, "urls_crawled_at", 0, 0).Result(); err == nil && len(oldest) > 0 {
		ageSeconds := time.Now().Unix() - int64(oldest[0].Score)
		stats["oldest_crawled_age_seconds"] = fmt.Sprintf("%d", ageSeconds)
	}

	return stats, nil
}

// CloseRedis ferme la connexion à Redis
func CloseRedis() {
	if RedisClient != nil {
		if err := RedisClient.Close(); err != nil {
			log.Printf("Error closing Redis connection: %v", err)
		}
	}
}
