package main

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/internal/scraper"
	"github.com/joho/godotenv"
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
    ticker := time.NewTicker(time.Duration(DefaultInterval) * time.Minute)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            log.Printf("⏰ Exécution programmée du scraper à %s", time.Now().Format("15:04:05"))
            scraper.ScrapEverest()
        }
    }
}

// func scrapAndStore_DeepDarkCTI() {
//     dbName := utils.GetEnvOrDefault("MONGO_DB", "BlueWhiteThreat")
//     entries, err := deepdarkCTI.ScrapeAll()
//     if err != nil {
//         log.Fatalf("Erreur lors du scrape : %v", err)
//     }
//     err = db.SaveAllEntries(dbName, "deepdarkCTI", entries)
//     if err != nil {
//         log.Fatalf("MongoDB error: %v", err)
//     } else {
//         log.Printf("Scrap sauvegardé dans MongoDB à %s", time.Now().Format(time.RFC3339))
//     }
// }

func scrapAll() {

}
