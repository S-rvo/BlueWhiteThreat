package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/crawler/internal/api"
	"github.com/S-rvo/BlueWhiteThreat/apps/crawler/internal/crawler"
	"github.com/S-rvo/BlueWhiteThreat/apps/crawler/internal/db"
	"github.com/S-rvo/BlueWhiteThreat/apps/crawler/internal/seeddata"
	"github.com/S-rvo/BlueWhiteThreat/pkg/csamfilter"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// Initialisation des bases de données
	initDatabases()
	defer db.CloseRedis()
	defer db.CloseNeo4j()

	// Lancer l'API en parallèle
	go startAPI(ctx)

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
	startCrawler(ctx)
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
func startAPI(ctx context.Context) {

	router := api.NewRouter()
	srv := &http.Server{
		Addr:         ":8080",
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	log.Println("API listening at http://localhost:8080")
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(c)
	}()
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("API: %v", err)
	}
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
func startCrawler(ctx context.Context) {
	workers := 4
	if n, err := strconv.Atoi(os.Getenv("CRAWLER_WORKERS")); err == nil && n >= 1 && n <= 32 {
		workers = n
	}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(sweep bool) { defer wg.Done(); crawlerWorker(ctx, sweep) }(i == 0)
	}
	wg.Wait()
}
func crawlerWorker(ctx context.Context, sweep bool) {
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

	for ctx.Err() == nil {
		maybeRequeueStale()
		if sweep {
			if err := db.DrainRenderedLinks(); err != nil {
				log.Printf("rendered links: %v", err)
			}
		}

		url, token, err := db.ClaimURL()
		if err != nil {
			log.Printf("Claim failed: %v", err)
			pause(ctx, time.Second)
			continue
		}
		if url == "" {
			pause(ctx, time.Second)
			continue
		}

		// Beaucoup d'URLs ont été mises en queue avant l'ajout du filtre
		// de blocage (queue persistante dans Redis) : il faut donc
		// re-vérifier ici, au moment de dépiler, pas seulement à
		// l'ajout, sinon ces entrées historiques sont crawlées en
		// direct sur Tor sans aucun contrôle.
		if csamfilter.IsBlockedURL(url) || db.IsFarmBackend(url) {
			if err := db.DropClaim(url, token); err != nil {
				log.Printf("Error dropping blocked URL: %v", err)
			}
			continue
		}

		processURL(ctx, url, token)
		pause(ctx, 500*time.Millisecond)
	}
}

// releaseForRetry relâche la lease en échec : la politesse hôte est rendue,
// le compteur de tentatives avance, le dead-letter reste atteignable.
func releaseForRetry(url, token string) {
	if e := db.FinishClaim(url, token, false, db.RetryDelay(url)); e != nil {
		log.Printf("release: %v", e)
	}
}

// processURL traite une URL spécifique : crawling + stockage
func processURL(parent context.Context, url, token string) {
	workCtx, cancel := context.WithCancel(parent)
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
		releaseForRetry(url, token)
		return
	}
	fetchStarted := time.Now()
	result, err := crawler.FetchContext(workCtx, url, metadata)
	if e := db.RecordHostFetch(url, result.Status, time.Since(fetchStarted), err != nil); e != nil {
		log.Printf("host metrics: %v", e)
	}
	if workCtx.Err() != nil {
		return
	}
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
		releaseForRetry(url, token)
		return
	}
	acceptedLinks := make([]string, 0, len(result.Links))
	for _, link := range result.Links {
		if workCtx.Err() != nil {
			return
		}
		reason, err := db.AddURLToQueue(link)
		if err != nil {
			log.Printf("enqueue: %v", err)
			releaseForRetry(url, token)
			return
		}

		if reason != "storage" && reason != "trap" && reason != db.QueueReasonBlocked && reason != db.QueueReasonFarm {
			acceptedLinks = append(acceptedLinks, link)
		}
	}
	if err := db.SaveLinks(url, acceptedLinks); err != nil {
		log.Printf("graph: %v", err)
		releaseForRetry(url, token)
		return
	}
	interval := crawler.RevisitInterval(result, metadata)
	if result.Status != http.StatusNotModified {
		if err := db.SaveClaimMetadata(url, token, result.ETag, result.Modified, result.Hash); err != nil {
			log.Printf("metadata: %v", err)
			releaseForRetry(url, token)
			return
		}
	}
	if err := db.FinishClaim(url, token, true, interval, result.Status); err != nil {
		log.Printf("ack: %v", err)
	}
}

func pause(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
