package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/crawler/internal/api"
	"github.com/S-rvo/BlueWhiteThreat/apps/crawler/internal/crawler"
	"github.com/S-rvo/BlueWhiteThreat/apps/crawler/internal/db"
	"github.com/S-rvo/BlueWhiteThreat/apps/crawler/internal/seeddata"
	"github.com/S-rvo/BlueWhiteThreat/pkg/csamfilter"
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
	// Seeds additionnels : adresses de sites de leak ransomware connus,
	// issues d'une source CTI clearnet publique (ThreatCluster, 2026-11-06),
	// nécessaires car le seed unique précédent a un graphe de liens déjà
	// entièrement exploré (voir pistes-a-creuser point 11).
	for _, line := range strings.Split(seeddata.SeedURLsFile, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			startURLs = append(startURLs, line)
		}
	}
	for _, url := range startURLs {
		if _, err := db.AddURLToQueue(url); err != nil {
			log.Printf("Error adding start URL to queue: %v", err)
		}
	}

	// Démarrer le crawler
	startCrawler(0)
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
	if err := db.InitNeo4jDriver(
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

// Sans la puissance de calcul de Google, la fraîcheur du graphe de liens ne
// peut venir que d'un re-crawl périodique, borné, du contenu déjà visité
// (voir pistes-a-creuser point 11) : un sweep modeste plutôt qu'un balayage
// massif qu'on n'a pas les ressources Tor pour absorber.
const (
	staleRequeueMaxAge = 7 * 24 * time.Hour
	staleRequeueLimit  = 200
	staleRequeueEvery  = 30 * time.Minute
	idleRetryInterval  = 2 * time.Minute
)

// startCrawler gère la boucle principale de crawling. Elle ne se termine
// jamais : une file vide est un état transitoire (tout le graphe connu a été
// visité récemment), pas une fin de tâche -- contrairement à l'ancien
// comportement qui faisait sortir le process et comptait sur
// `restart: unless-stopped` pour "relancer" une queue déjà vide.
func startCrawler(maxDepth int) {
	workers := 4
	if n, err := strconv.Atoi(os.Getenv("CRAWLER_WORKERS")); err == nil && n >= 1 && n <= 32 {
		workers = n
	}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(sweep bool) { defer wg.Done(); crawlerWorker(maxDepth, sweep) }(i == 0)
	}
	wg.Wait()
}
func crawlerWorker(maxDepth int, sweep bool) {
	lastStaleSweep := time.Now()

	maybeRequeueStale := func() {
		if !sweep || time.Since(lastStaleSweep) < staleRequeueEvery {
			return
		}
		lastStaleSweep = time.Now()
		requeued, err := db.RequeueStaleURLs(staleRequeueMaxAge, staleRequeueLimit)
		if err != nil {
			log.Printf("Error requeuing stale URLs: %v", err)
			return
		}
		if requeued > 0 {
			log.Printf("Requeued %d stale URLs for re-crawl", requeued)
		}
	}

	for {
		maybeRequeueStale()

		url, token, err := db.ClaimURL()
		if err != nil {
			log.Printf("Claim failed: %v", err)
			time.Sleep(time.Second)
			continue
		}
		if url == "" {
			time.Sleep(time.Second)
			continue
		}

		// Beaucoup d'URLs ont été mises en queue avant l'ajout du filtre
		// de blocage (queue persistante dans Redis) : il faut donc
		// re-vérifier ici, au moment de dépiler, pas seulement à
		// l'ajout, sinon ces entrées historiques sont crawlées en
		// direct sur Tor sans aucun contrôle.
		if csamfilter.IsBlockedURL(url) || db.IsFarmBackend(url) {
			if err := db.FinishClaim(url, token, true, 30*24*time.Hour, http.StatusForbidden); err != nil {
				log.Printf("Error marking blocked URL as crawled: %v", err)
			}
			continue
		}

		processURL(url, token, maxDepth)
		time.Sleep(500 * time.Millisecond)
	}
}

// processURL traite une URL spécifique : crawling + stockage
func processURL(url, token string, maxDepth int) {
	workCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := time.Now()
	defer func() {
		if err := db.RecordProcessingDuration(time.Since(started)); err != nil {
			log.Printf("metrics: %v", err)
		}
	}()
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				if err := db.RenewClaim(url, token); err != nil {
					log.Printf("lease renewal: %v", err)
					cancel()
					return
				}
			}
		}
	}()
	defer func() { close(stop); <-done }()
	metadata, err := db.FetchMetadata(url)
	if err != nil {
		log.Printf("metadata: %v", err)
		return
	}
	result, err := crawler.FetchContext(workCtx, url, metadata)
	if err != nil {
		delay := db.RetryDelay(url)
		if result.RetryAfter > delay {
			delay = result.RetryAfter
		}
		if e := db.FinishClaim(url, token, false, delay); e != nil {
			log.Printf("retry: %v", e)
		}
		log.Printf("fetch %s: %v", url, err)
		return
	}
	if workCtx.Err() != nil {
		return
	}
	// Persist discovery before acknowledgment so a storage failure is replayed.
	if err := db.SaveURL(url); err != nil {
		log.Printf("graph: %v", err)
		return
	}
	for _, link := range result.Links {
		if workCtx.Err() != nil {
			return
		}
		if _, err := db.AddURLToQueue(link); err != nil {
			log.Printf("enqueue: %v", err)
			return
		}

	}
	if err := db.SaveLinks(url, result.Links); err != nil {
		log.Printf("graph: %v", err)
		return
	}
	interval := crawler.RevisitInterval(result, metadata)
	if result.Status != http.StatusNotModified {
		if err := db.SaveClaimMetadata(url, token, result.ETag, result.Modified, result.Hash); err != nil {
			log.Printf("metadata: %v", err)
			return
		}
	}
	if err := db.FinishClaim(url, token, true, interval, result.Status); err != nil {
		log.Printf("ack: %v", err)
	}
}
