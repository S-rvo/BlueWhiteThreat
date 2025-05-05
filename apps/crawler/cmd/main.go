package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/internal/crawler"
	"github.com/S-rvo/BlueWhiteThreat/internal/db"
)

// URLs de départ à crawler
var startURLs = []string{
	"http://6nhmgdpnyoljh5uzr5kwlatx2u3diou4ldeommfxjz3wkhalzgjqxzqd.onion",
}

func main() {
	// Initialiser les connexions aux bases de données
	if err := db.InitRedis(); err != nil {
		log.Fatalf("Error initializing Redis: %v", err)
	}
	defer db.CloseRedis()

	// Configuration initiale de Redis
	if err := db.SetupRedisQueues(); err != nil {
		log.Fatalf("Error setting up Redis queues: %v", err)
	}

	// Ajouter les URLs de départ à la file d'attente Redis
	for _, url := range startURLs {
		if err := db.AddURLToQueue(url); err != nil {
			log.Printf("Error adding start URL to queue: %v", err)
		}
	}

	// Configuration pour capturer Ctrl+C
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		fmt.Println("\nShutting down gracefully...")
		db.CloseRedis()
		os.Exit(0)
	}()

	// Configuration du crawler
	maxDepth := 1

	// Boucle principale du crawler - continue jusqu'à ce que la file d'attente soit vide
	for {
		// Récupérer la taille de la file d'attente
		queueSize, err := db.GetQueueSize()
		if err != nil {
			log.Fatalf("Error getting queue size: %v", err)
		}

		// Si la file est vide, quitter la boucle
		if queueSize == 0 {
			log.Println("Queue is empty. Crawler has completed.")
			break
		}

		// Récupérer la prochaine URL à crawler
		url, err := db.GetNextURLFromQueue()
		if err != nil {
			log.Printf("Error getting next URL from queue: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		// Si aucune URL n'a été retournée (même si la queue n'est pas vide d'après SCard)
		if url == "" {
			log.Println("No URL returned from queue, but queue is not empty. Retrying...")
			time.Sleep(1 * time.Second)
			continue
		}

		log.Printf("Crawling URL: %s", url)

		// Crawler l'URL
		_, links, statusCode, err := crawler.Crawler(url, maxDepth)
		if err != nil {
			log.Printf("Error crawling URL %s: %v", url, err)
			// Même en cas d'erreur, marquer l'URL comme crawlée pour éviter les boucles
			if err := db.MarkURLAsCrawled(url); err != nil {
				log.Printf("Error marking URL as crawled: %v", err)
			}
			continue
		}

		// Traitement des résultats
		log.Printf("Crawled URL: %s - Status: %d, Found %d links", url, statusCode, len(links))

		// Marquer l'URL comme visitée
		if err := db.MarkURLAsCrawled(url); err != nil {
			log.Printf("Error marking URL as crawled: %v", err)
		}

		// Ajouter les nouvelles URLs découvertes à la file d'attente
		for _, newURL := range links {
			if err := db.AddURLToQueue(newURL); err != nil {
				log.Printf("Error adding new URL to queue: %v", err)
			}
		}

		// Afficher les statistiques courantes
		crawled, _ := db.GetCrawledCount()
		queued, _ := db.GetQueueSize()
		log.Printf("Stats - Crawled: %d, Queued: %d", crawled, queued)

		// Petite pause pour ne pas surcharger le système
		time.Sleep(500 * time.Millisecond)
	}

	// Afficher les statistiques finales
	crawled, _ := db.GetCrawledCount()
	log.Printf("Crawling complete! Total URLs crawled: %d", crawled)
}
