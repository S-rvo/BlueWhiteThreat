package db

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"sort"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/pkg/contract"
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

	const migrationKey = "crawler:migration:frontier_v1"
	if done, err := RedisClient.Exists(ctx, migrationKey).Result(); err != nil || done == 1 {
		return err
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

	if err := ReconcileFrontier(); err != nil {
		return err
	}
	return RedisClient.Set(ctx, migrationKey, 1, 0).Err()
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
// Google, les quelques workers de ce projet (à ~2 URLs/s chacun) ne peuvent
// pas absorber une file qui grossirait sans limite (seeds + liens découverts
// + requeue de fraîcheur). Au-delà, les nouveaux ajouts sont silencieusement
// différés durablement dans crawler:scheduled pour éviter de perdre les liens.
const (
	maxQueueSize    = 5000
	maxReadyPerHost = 64
)

// alertCooldownKeyPrefix préfixe les clés Redis utilisées par
// TryMarkAlertSent pour que le cooldown d'alerte (cmd/alerting) survive un
// redémarrage du conteneur, sans introduire de nouvelle structure dédiée.
const alertCooldownKeyPrefix = "alerting:cooldown:"

// TryMarkAlertSent renvoie true si l'alerte `condition` peut être envoyée
// maintenant (et marque son cooldown dans Redis via une clé à expiration),
// false si une alerte pour cette même condition a déjà été envoyée pendant
// la fenêtre `cooldown`. Le marquage est atomique (SET ... NX) : utile le
// jour où plusieurs instances d'alerting tournent en parallèle.
func TryMarkAlertSent(condition string, cooldown time.Duration) (bool, error) {
	key := alertCooldownKeyPrefix + condition
	ok, err := RedisClient.SetNX(ctx, key, time.Now().Unix(), cooldown).Result()
	if err != nil {
		return false, err
	}
	return ok, nil
}

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

// Raisons renvoyées par AddURLToQueue, pour que les appelants (notamment
// collectseeds, qui traite un lot borné et veut savoir pourquoi la file n'a
// presque pas grossi) puissent distinguer un ajout réel d'un no-op -- et
// pourquoi. Avant cette classification, les trois cas de no-op (CSAM, ferme,
// dédup) étaient indiscernables depuis les logs : un lot de seeds presque
// entièrement rejeté ne laissait aucune trace de la raison.
const (
	QueueReasonAdded     = "added"
	QueueReasonBlocked   = "blocked"
	QueueReasonFarm      = "farm"
	QueueReasonCapacity  = "capacity"
	QueueReasonDuplicate = "duplicate"
)

// AddURLToQueue ajoute une URL à la file d'attente si elle n'a pas déjà été
// traitée, et renvoie la raison du résultat (voir QueueReason*).
func AddURLToQueue(ctx context.Context, url string) (string, error) {
	cleaned := cleanOnionURL(url)
	_, valid := discoveryKey(cleaned)
	host, hostErr := HostKey(cleaned)
	if !valid || hostErr != nil {
		return "trap", nil
	}

	if csamfilter.IsBlockedURL(cleaned) {
		return QueueReasonBlocked, nil
	}
	if IsFarmBackend(cleaned) {
		return QueueReasonFarm, nil
	}

	return RedisClient.Eval(ctx, indexURLLua+scheduledHostsLua+`
 if redis.call('SISMEMBER',KEYS[1],ARGV[1])==1 or redis.call('SISMEMBER',KEYS[2],ARGV[1])==1 or redis.call('ZSCORE',KEYS[3],ARGV[1]) or redis.call('ZSCORE',KEYS[4],ARGV[1]) or redis.call('ZSCORE','crawler:dead',ARGV[1]) or redis.call('ZSCORE','crawler:blocked',ARGV[1]) or redis.call('ZSCORE','crawler:gone',ARGV[1]) then return 'duplicate' end

 if redis.call('HEXISTS','crawler:url_host',ARGV[1])==0 then
 if redis.call('HLEN','crawler:url_host')>=1000000 or tonumber(redis.call('HGET','crawler:stored_per_host',ARGV[4]) or '0')>=50000 then
 redis.call('HINCRBY','crawler:stats','storage_rejected_total',1); return 'storage' end
 redis.call('HSET','crawler:url_host',ARGV[1],ARGV[4])
 redis.call('HINCRBY','crawler:stored_per_host',ARGV[4],1)
 end
 local budget='crawler:discovery_daily:'..ARGV[4]..':'..math.floor(tonumber(ARGV[3])/86400)
 if tonumber(redis.call('GET',budget) or '0')>=10000 then
 scheduleURL(KEYS[3],ARGV[1],(math.floor(tonumber(ARGV[3])/86400)+1)*86400)
 redis.call('HINCRBY','crawler:stats','budget_deferred_total',1); return 'budget' end
 redis.call('INCR',budget); redis.call('EXPIRE',budget,172800)
 if redis.call('SCARD',KEYS[1])>=tonumber(ARGV[2]) or redis.call('ZCARD','crawler:host_queue:'..ARGV[4])>=tonumber(ARGV[5]) then
 scheduleURL(KEYS[3],ARGV[1],ARGV[3]); redis.call('HINCRBY','crawler:stats','deferred_total',1); return 'capacity' end
 redis.call('SADD',KEYS[1],ARGV[1]); indexURL(ARGV[1],ARGV[3]); redis.call('HINCRBY','crawler:stats','urls_queued',1); return 'added'
 `, []string{"urls_to_crawl", "urls_crawled", "crawler:scheduled", "crawler:inflight"}, cleaned, maxQueueSize, time.Now().Unix(), host, maxReadyPerHost).Text()

}

// queuePriority only orders the frontier; the rules are shared with the
// scraper through pkg/contract.
func queuePriority(rawURL string) int {
	return contract.URLPriority(rawURL)
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

// RequeueStaleURLs re-surfaces already-crawled URLs whose last visit is older
// than maxAge, so the frontier doesn't go permanently stale once the backlog
// of brand-new URLs runs out. Bounded by limit: without Google's crawl
// budget, a sweep must stay small per call rather than requeuing everything
// stale at once. High-CTI-signal URLs are requeued first.
func RequeueStaleURLs(ctx context.Context, maxAge time.Duration, limit int64) (int64, error) {
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

	var requeued int64
	for _, url := range stale {
		added, err := RedisClient.Eval(ctx, indexURLLua+`
 if redis.call('SCARD',KEYS[1])>=tonumber(ARGV[2]) then return 0 end
 if redis.call('ZSCORE',KEYS[2],ARGV[1]) or redis.call('ZSCORE',KEYS[3],ARGV[1]) or redis.call('ZSCORE','crawler:dead',ARGV[1]) or redis.call('ZSCORE','crawler:blocked',ARGV[1]) or redis.call('ZSCORE','crawler:gone',ARGV[1]) then return 0 end
 local host=redis.call('HGET','crawler:url_host',ARGV[1]) or string.match(ARGV[1],'^https?://([^/?#]+)')
 if not host or redis.call('ZCARD','crawler:host_queue:'..host)>=tonumber(ARGV[3]) then return 0 end
 local added=redis.call('SADD',KEYS[1],ARGV[1]); indexURL(ARGV[1],0); return added`, []string{"urls_to_crawl", "crawler:scheduled", "crawler:inflight"}, url, maxQueueSize, maxReadyPerHost).Int64()
		if err != nil {
			return requeued, err
		}
		requeued += added
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

	now := time.Now().Unix()
	pipe := RedisClient.Pipeline()
	due := pipe.ZCount(ctx, "crawler:scheduled", "-inf", fmt.Sprint(now))
	oldestDue := pipe.ZRangeWithScores(ctx, "crawler:scheduled", 0, 0)
	counts := map[string]*redis.IntCmd{}
	for name, key := range map[string]string{
		"inflight_count":  "crawler:inflight",
		"scheduled_count": "crawler:scheduled",
		"dead_count":      "crawler:dead",
		"gone_count":      "crawler:gone",
		"blocked_count":   "crawler:blocked",
	} {
		counts[name] = pipe.ZCard(ctx, key)
	}
	_, _ = pipe.Exec(ctx)
	for name, cmd := range counts {
		if n, err := cmd.Result(); err == nil {
			stats[name] = fmt.Sprint(n)
		}
	}
	if n, err := due.Result(); err == nil {
		stats["scheduled_due_count"] = fmt.Sprint(n)
		if total, err := counts["scheduled_count"].Result(); err == nil {
			stats["scheduled_future_count"] = fmt.Sprint(max(int64(0), total-n))
		}
	}
	if scores, err := oldestDue.Result(); err == nil && len(scores) > 0 {
		stats["oldest_due_age_seconds"] = fmt.Sprint(max(int64(0), now-int64(scores[0].Score)))
	}

	if n, err := RedisClient.HLen(ctx, "crawler:url_host").Result(); err == nil {
		stats["known_url_count"] = fmt.Sprint(n)
	}
	stats["known_url_limit"] = "1000000"
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
