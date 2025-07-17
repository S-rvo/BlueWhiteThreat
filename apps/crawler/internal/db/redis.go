package db

import (
	"context"
	"log"
	"os"
	"time"

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

	if err := RedisClient.FlushDB(ctx).Err(); err != nil {
		return err
	}
	log.Println("Redis database flushed on startup")
	log.Printf("Connected to Redis: %s:%s", redisHost, redisPort)
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

	return nil
}

// AddURLToQueue ajoute une URL à la file d'attente si elle n'a pas déjà été traitée
func AddURLToQueue(url string) error {
	cleaned := cleanOnionURL(url)

	pipe := RedisClient.Pipeline()
	isCrawledCmd := pipe.SIsMember(ctx, "urls_crawled", cleaned)
	isQueuedCmd := pipe.SIsMember(ctx, "urls_to_crawl", cleaned)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return err
	}

	isCrawled, _ := isCrawledCmd.Result()
	isQueued, _ := isQueuedCmd.Result()

	if !isCrawled && !isQueued {
		err = RedisClient.SAdd(ctx, "urls_to_crawl", cleaned).Err()
		if err != nil {
			return err
		}
		RedisClient.HIncrBy(ctx, "crawler:stats", "urls_queued", 1)
		log.Printf("Added URL to queue: %s", cleaned)
	}

	return nil
}

// GetNextURLFromQueue récupère une URL de la file d'attente et la supprime
func GetNextURLFromQueue() (string, error) {
	url, err := RedisClient.SPop(ctx, "urls_to_crawl").Result()
	if err != nil {
		if err == redis.Nil {
			return "", nil
		}
		return "", err
	}
	return url, nil
}

// MarkURLAsCrawled marque une URL comme visitée
func MarkURLAsCrawled(url string) error {
	cleaned := cleanOnionURL(url)
	err := RedisClient.SAdd(ctx, "urls_crawled", cleaned).Err()
	if err != nil {
		return err
	}
	RedisClient.HIncrBy(ctx, "crawler:stats", "urls_crawled", 1)
	return nil
}

// GetQueueSize renvoie le nombre d'URLs dans la file d'attente
func GetQueueSize() (int64, error) {
	return RedisClient.SCard(ctx, "urls_to_crawl").Result()
}

// GetCrawledCount renvoie le nombre d'URLs crawlées
func GetCrawledCount() (int64, error) {
	return RedisClient.SCard(ctx, "urls_crawled").Result()
}

// FetchCrawlerStats récupère les statistiques du crawler depuis Redis
func FetchCrawlerStats() (map[string]string, error) {
	stats, err := RedisClient.HGetAll(ctx, "crawler:stats").Result()
	if err != nil {
		return nil, err
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
