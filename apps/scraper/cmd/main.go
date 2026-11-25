package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/db"
	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/sites"
	"github.com/joho/godotenv"
)

var DefaultInterval = 30 // 30 min par défaut

func main() {
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

	if i, err := strconv.Atoi(os.Getenv("SCRAPER_INTERVAL_MIN")); err == nil && i > 0 {
		DefaultInterval = i
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

	log.Printf("Lancement du scraper (interval: %d min)", DefaultInterval)
	ticker := time.NewTicker(time.Duration(DefaultInterval) * time.Minute)
	defer ticker.Stop()
	for ctx.Err() == nil {
		sites.RunDarkwebScraper(ctx, client)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
