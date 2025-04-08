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
    // Récupérer les paramètres de connexion à partir des variables d'environnement
    redisHost := os.Getenv("REDIS_HOST")
    if redisHost == "" {
        redisHost = "localhost"
    }

    redisPort := os.Getenv("REDIS_PORT")
    if redisPort == "" {
        redisPort = "6379"
    }

    // Connexion à Redis
    RedisClient = redis.NewClient(&redis.Options{
        Addr:     redisHost + ":" + redisPort,
        Password: os.Getenv("REDIS_PASSWORD"), // Pas de mot de passe par défaut
        DB:       0,                           // Utiliser la base de données 0
    })

    // Vérifier la connexion
    _, err := RedisClient.Ping(ctx).Result()
    if err != nil {
        return err
    }

    log.Printf("Connected to Redis: %s:%s", redisHost, redisPort)
    return nil
}

// CloseRedis ferme la connexion à Redis
func CloseRedis() {
    if RedisClient != nil {
        if err := RedisClient.Close(); err != nil {
            log.Printf("Error closing Redis connection: %v", err)
        }
    }
}

// SetupRedisQueues initialise les structures de données nécessaires dans Redis
func SetupRedisQueues() error {
    // Vérifier et créer les ensembles pour le crawler
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

    // Initialiser les compteurs si nécessaire
    if exists, err := RedisClient.Exists(ctx, "crawler:stats").Result(); err != nil {
        return err
    } else if exists == 0 {
        log.Println("Initializing crawler:stats hash in Redis")
        RedisClient.HSet(ctx, "crawler:stats", map[string]interface{}{
            "urls_crawled": 0,
            "urls_queued": 0,
            "start_time": time.Now().Unix(),
        })
    }

    return nil
}

// AddURLToQueue ajoute une URL à la file d'attente si elle n'a pas déjà été traitée
func AddURLToQueue(url string) error {
    // Vérification complète en une seule opération : 
    // 1. L'URL n'est pas déjà crawlée
    // 2. L'URL n'est pas déjà dans la file d'attente
    
    pipe := RedisClient.Pipeline()
    isCrawledCmd := pipe.SIsMember(ctx, "urls_crawled", url)
    isQueuedCmd := pipe.SIsMember(ctx, "urls_to_crawl", url)
    _, err := pipe.Exec(ctx)
    if err != nil {
        return err
    }
    
    isCrawled, _ := isCrawledCmd.Result()
    isQueued, _ := isQueuedCmd.Result()

    // Si l'URL n'a pas été crawlée et n'est pas déjà en file d'attente, l'ajouter
    if !isCrawled && !isQueued {
        err = RedisClient.SAdd(ctx, "urls_to_crawl", url).Err()
        if err != nil {
            return err
        }
        
        // Mettre à jour les statistiques
        RedisClient.HIncrBy(ctx, "crawler:stats", "urls_queued", 1)
        log.Printf("Added URL to queue: %s", url)
    }

    return nil
}

// GetNextURLFromQueue récupère une URL de la file d'attente et la supprime
func GetNextURLFromQueue() (string, error) {
    // Utiliser SPOP pour retirer et renvoyer un élément aléatoire de l'ensemble
    url, err := RedisClient.SPop(ctx, "urls_to_crawl").Result()
    if err != nil {
        if err == redis.Nil {
            // Aucune URL dans la file d'attente
            return "", nil
        }
        return "", err
    }
    
    return url, nil
}

// MarkURLAsCrawled marque une URL comme visitée
func MarkURLAsCrawled(url string) error {
    // Ajouter l'URL à l'ensemble des URLs crawlées
    err := RedisClient.SAdd(ctx, "urls_crawled", url).Err()
    if err != nil {
        return err
    }
    
    // Mettre à jour les statistiques
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
