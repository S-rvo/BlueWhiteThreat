package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/internal/api"
	"github.com/S-rvo/BlueWhiteThreat/internal/crawler"
	"github.com/S-rvo/BlueWhiteThreat/internal/db"
)

func main() {
	// Initialisation des bases de données
	initDatabases()
	defer db.CloseRedis()
	defer db.CloseNeo4j()

	// Lancer l'API en parallèle
	go startAPI()

	// Ajouter les URLs de départ
	startURLs := []string{
		"http://6nhmgdpnyoljh5uzr5kwlatx2u3diou4ldeommfxjz3wkhalzgjqxzqd.onion",
	}
	for _, url := range startURLs {
		if err := db.AddURLToQueue(url); err != nil {
			log.Printf("Error adding start URL to queue: %v", err)
		}
	}

	// Démarrer le crawler
	startCrawler(1)
}

// initDatabases initialise Redis et Neo4j
func initDatabases() {
	// Initialiser Redis
	if err := db.InitRedis(); err != nil {
		log.Fatalf("Error initializing Redis: %v", err)
	}

	// Configurer Redis
	if err := db.SetupRedisQueues(); err != nil {
		log.Fatalf("Error setting up Redis queues: %v", err)
	}

	// Initialiser Neo4j
	if err := db.InitNeo4j(
		os.Getenv("NEO4J_URI"),
		os.Getenv("NEO4J_USER"),
		os.Getenv("NEO4J_PASSWORD"),
	); err != nil {
		log.Fatalf("Error initializing Neo4j: %v", err)
	}
}

// startAPI lance l'API HTTP
func startAPI() {

	router := api.NewRouter()
	srv := &http.Server{
		Addr:         ":8080",
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	log.Println("API listening at http://localhost:8080")
	log.Fatal(srv.ListenAndServe())
}

// startCrawler gère la boucle principale de crawling
func startCrawler(maxDepth int) {
	for {
		queueSize, err := db.GetQueueSize()
		if err != nil {
			log.Fatalf("Error getting queue size: %v", err)
		}

		if queueSize == 0 {
			log.Println("Queue is empty. Crawler has completed.")
			break
		}

		url, err := db.GetNextURLFromQueue()
		if err != nil {
			log.Printf("Error getting next URL from queue: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}
		if url == "" {
			log.Println("No URL returned from queue, but queue is not empty. Retrying...")
			time.Sleep(1 * time.Second)
			continue
		}

		processURL(url, maxDepth)
		time.Sleep(500 * time.Millisecond)
	}

	crawled, _ := db.GetCrawledCount()
	log.Printf("Crawling complete! Total URLs crawled: %d", crawled)
}

// processURL traite une URL spécifique : crawling + stockage
func processURL(url string, maxDepth int) {
	log.Printf("Crawling URL: %s", url)

	_, links, statusCode, err := crawler.Crawler(url, maxDepth)
	if err != nil {
		log.Printf("Error crawling URL %s: %v", url, err)
		db.MarkURLAsCrawled(url)
		return
	}

	log.Printf("Crawled URL: %s - Status: %d, Found %d links", url, statusCode, len(links))

	// Marquer l'URL comme crawlée
	if err := db.MarkURLAsCrawled(url); err != nil {
		log.Printf("Error marking URL as crawled: %v", err)
	}

	// Sauvegarder l'URL de départ dans Neo4j
	if err := db.SaveURL(url); err != nil {
		log.Printf("Error saving URL in Neo4j: %v", err)
	}

	// Traiter les liens trouvés
	for _, newURL := range links {
		// Ajouter en queue Redis
		if err := db.AddURLToQueue(newURL); err != nil {
			log.Printf("Error adding new URL to queue: %v", err)
		}
		// Enregistrer la relation Neo4j
		if err := db.SaveLink(url, newURL); err != nil {
			log.Printf("Error saving link in Neo4j: %v", err)
		}
	}

	crawled, _ := db.GetCrawledCount()
	queued, _ := db.GetQueueSize()
	log.Printf("Stats - Crawled: %d, Queued: %d", crawled, queued)
}
