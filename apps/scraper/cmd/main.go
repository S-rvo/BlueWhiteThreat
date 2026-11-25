package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/db"
	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/sites"
	"github.com/joho/godotenv"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--check-health" {
		client := &http.Client{Timeout: 4 * time.Second}
		resp, err := client.Get("http://127.0.0.1:8080/readyz")
		if err != nil {
			log.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			log.Fatalf("scraper not ready (HTTP %d)", resp.StatusCode)
		}
		return
	}
	checkOnly := len(os.Args) == 2 && os.Args[1] == "--check-browser-sandbox"
	if checkOnly || os.Getenv("SCRAPER_REQUIRE_SANDBOX") == "1" {
		if err := sites.CheckBrowserSandbox(context.Background()); err != nil {
			log.Fatalf("Scraper isolation: %v", err)
		}
		log.Print("Scraper isolation: Chromium namespaces and seccomp active")
		if checkOnly {
			return
		}
	}
	// Load the private key file first; existing process environment remains authoritative.
	if err := godotenv.Load(".credentials.env"); err != nil && !os.IsNotExist(err) {
		log.Fatal("Lecture du fichier de clé privée échouée.")
	}
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		log.Print("Lecture du fichier .env échouée.")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	client, err := db.ConnectMongo()
	if err != nil {
		log.Fatalf("Initialisation Mongo: %v", err)
	}
	defer func() {
		closing, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.Disconnect(closing)
	}()

	redisClient := db.ConnectRedis()
	defer redisClient.Close()

	server := &http.Server{
		Addr:              ":8080",
		Handler:           serviceHandler(redisClient, client),
		ReadHeaderTimeout: 3 * time.Second,
		WriteTimeout:      6 * time.Second,
		IdleTimeout:       15 * time.Second,
	}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP scraper: %v", err) // restart policy reopens the service
		}
	}()
	log.Print("Scraper continu actif")
	sites.RunDarkwebScraper(ctx, redisClient, client)
	closing, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = server.Shutdown(closing)
}
