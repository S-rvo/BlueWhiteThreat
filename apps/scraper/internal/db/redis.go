package db

import (
	"context"
	"os"

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
	return client.SDiff(ctx, "urls_crawled", "urls_scraped").Result()
}

// MarkURLScraped marque une URL comme traitée pour ne pas la retraiter au tick suivant.
func MarkURLScraped(ctx context.Context, client *redis.Client, url string) error {
	return client.SAdd(ctx, "urls_scraped", url).Err()
}
