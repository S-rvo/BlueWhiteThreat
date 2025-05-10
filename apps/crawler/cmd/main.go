package main

import (
	"log"
	"net/http"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/internal/api"
	"github.com/S-rvo/BlueWhiteThreat/internal/crawler"
	"github.com/S-rvo/BlueWhiteThreat/internal/db"
)

func main() {
	// Initialiser Redis
	if err := db.InitRedis(); err != nil {
		log.Fatalf("Error initializing Redis: %v", err)
	}
	defer db.CloseRedis()

	// Configuration initiale de Redis
	if err := db.SetupRedisQueues(); err != nil {
		log.Fatalf("Error setting up Redis queues: %v", err)
	}
	// Initialiser l'api
	router := api.NewRouter()
	log.Println("API listening at http://localhost:8080")
	go func() {
		log.Fatal(http.ListenAndServe(":8080", router))
	}()

	// URL de depart
	startURLs := []string{
		"http://6nhmgdpnyoljh5uzr5kwlatx2u3diou4ldeommfxjz3wkhalzgjqxzqd.onion",
	}
	for _, url := range startURLs {
		if err := db.AddURLToQueue(url); err != nil {
			log.Printf("Error adding start URL to queue: %v", err)
		}
	}

	maxDepth := 1

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

		log.Printf("Crawling URL: %s", url)

		_, links, statusCode, err := crawler.Crawler(url, maxDepth)
		if err != nil {
			log.Printf("Error crawling URL %s: %v", url, err)
			if err := db.MarkURLAsCrawled(url); err != nil {
				log.Printf("Error marking URL as crawled: %v", err)
			}
			continue
		}

		log.Printf("Crawled URL: %s - Status: %d, Found %d links", url, statusCode, len(links))

		if err := db.MarkURLAsCrawled(url); err != nil {
			log.Printf("Error marking URL as crawled: %v", err)
		}

		for _, newURL := range links {
			if err := db.AddURLToQueue(newURL); err != nil {
				log.Printf("Error adding new URL to queue: %v", err)
			}
		}

		crawled, _ := db.GetCrawledCount()
		queued, _ := db.GetQueueSize()
		log.Printf("Stats - Crawled: %d, Queued: %d", crawled, queued)
		time.Sleep(500 * time.Millisecond)
	}

	crawled, _ := db.GetCrawledCount()
	log.Printf("Crawling complete! Total URLs crawled: %d", crawled)
}
