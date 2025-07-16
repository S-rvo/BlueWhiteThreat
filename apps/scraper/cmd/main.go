package main

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/sites"
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
			sites.DarkThreat()
		}
	}
}
