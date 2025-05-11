package main

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"

	"github.com/S-rvo/BlueWhiteThreat/internal/db"
	"github.com/S-rvo/BlueWhiteThreat/internal/deepdarkCTI"
	"github.com/S-rvo/BlueWhiteThreat/internal/utils"
)

var DefaultInterval = 30 // 30 min par défaut

func main() {
    err := godotenv.Load()
    if err != nil {
        log.Println("Aucun .env trouvé ou problème de lecture.")
    }

    if i, err := strconv.Atoi(os.Getenv("SCRAPER_INTERVAL_MIN")); err == nil && i > 0 {
        DefaultInterval = i
    }

    log.Printf("Lancement du scraper (interval: %d min)", DefaultInterval)
    timer := time.NewTicker(time.Duration(DefaultInterval) * time.Minute)
    defer timer.Stop()

    for {
        scrapAndStore_DeepDarkCTI()
        <-timer.C
    }
}

func scrapAndStore_DeepDarkCTI() {
    dbName := utils.GetEnvOrDefault("MONGO_DB", "BlueWhiteThreat")
    entries, err := deepdarkCTI.ScrapeAll()
    if err != nil {
        log.Fatalf("Erreur lors du scrape : %v", err)
    }
    err = db.SaveAllEntries(dbName, "deepdarkCTI", entries)
    if err != nil {
        log.Fatalf("MongoDB error: %v", err)
    } else {
        log.Printf("Scrap sauvegardé dans MongoDB à %s", time.Now().Format(time.RFC3339))
    }
}

