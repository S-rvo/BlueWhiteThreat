// alerting surveille périodiquement les métriques exposées par
// db.FetchCrawlerStats (déjà calculées à la volée, voir internal/db/redis.go)
// et pousse une notification webhook (format Slack "incoming webhook",
// {"text": "..."}) quand la file approche sa capacité ou que la fraîcheur du
// crawl dérive. Volontairement minimal : pas de Prometheus/Grafana/
// Alertmanager, juste un process borné qui vérifie quelques seuils -- cohérent
// avec le reste du projet (on n'a pas la puissance de calcul de Google pour
// opérer une stack d'observabilité complète).
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/crawler/internal/db"
)

const (
	// queueCapacityWarnRatio : alerte quand la file dépasse 90% de
	// maxQueueSize (voir db.QueueCapacityRatio) -- laisse une marge pour agir
	// avant que le cap de backpressure ne commence à ignorer des ajouts.
	queueCapacityWarnRatio = 0.9

	// staleAgeWarnThreshold : le sweep de fraîcheur (cmd/main, staleRequeueMaxAge)
	// requeue tout ce qui a plus de 7 jours et rafraîchit son timestamp. Si
	// l'entrée la plus ancienne dépasse le double de ce délai, c'est que le
	// sweep ne tourne plus (crawler down, bug) -- pas juste une file lente.
	staleAgeWarnThreshold = 14 * 24 * time.Hour

	defaultCheckInterval = 15 * time.Minute
	maxCheckInterval     = 6 * time.Hour

	// alertCooldown évite de spammer le webhook à chaque tick une fois un
	// seuil franchi. Gardé en mémoire du process (pas persisté dans Redis) :
	// un redémarrage du conteneur peut réémettre une alerte déjà envoyée,
	// compromis accepté pour rester simple.
	alertCooldown = 6 * time.Hour
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	intervalFlag := flag.Duration("interval", defaultCheckInterval, "intervalle entre deux vérifications (borné)")
	loop := flag.Bool("loop", false, "relance en continu au lieu de vérifier une seule fois")
	flag.Parse()

	interval := *intervalFlag
	if interval > maxCheckInterval {
		return fmt.Errorf("intervalle trop long (%s > %s)", interval, maxCheckInterval)
	}

	webhookURL := os.Getenv("ALERT_WEBHOOK_URL")
	if webhookURL == "" {
		log.Println("ALERT_WEBHOOK_URL non défini : les alertes seront seulement loguées, pas envoyées")
	}

	if err := db.InitRedis(); err != nil {
		return fmt.Errorf("connexion Redis: %w", err)
	}
	defer db.CloseRedis()

	lastSent := map[string]time.Time{}

	checkOnce := func() {
		if err := checkThresholds(webhookURL, lastSent); err != nil {
			log.Printf("Erreur vérification des seuils: %v", err)
		}
	}

	checkOnce()
	if !*loop {
		return nil
	}
	for {
		log.Printf("Prochaine vérification dans %s", interval)
		time.Sleep(interval)
		checkOnce()
	}
}

func checkThresholds(webhookURL string, lastSent map[string]time.Time) error {
	ratio, err := db.QueueCapacityRatio()
	if err != nil {
		return fmt.Errorf("lecture ratio de capacité: %w", err)
	}
	if ratio >= queueCapacityWarnRatio {
		maybeAlert(webhookURL, lastSent, "queue_capacity",
			fmt.Sprintf("Crawler: file d'attente à %.0f%% de sa capacité (seuil: %.0f%%)", ratio*100, queueCapacityWarnRatio*100))
	}

	stats, err := db.FetchCrawlerStats()
	if err != nil {
		return fmt.Errorf("lecture des stats: %w", err)
	}
	if raw, ok := stats["oldest_crawled_age_seconds"]; ok {
		ageSeconds, err := strconv.ParseInt(raw, 10, 64)
		if err == nil {
			age := time.Duration(ageSeconds) * time.Second
			if age > staleAgeWarnThreshold {
				maybeAlert(webhookURL, lastSent, "freshness",
					fmt.Sprintf("Crawler: l'URL la plus ancienne non re-crawlée date de %s (seuil: %s) -- le sweep de fraîcheur semble bloqué", age.Round(time.Hour), staleAgeWarnThreshold))
			}
		}
	}

	return nil
}

func maybeAlert(webhookURL string, lastSent map[string]time.Time, condition, message string) {
	if last, ok := lastSent[condition]; ok && time.Since(last) < alertCooldown {
		return
	}
	lastSent[condition] = time.Now()

	log.Printf("ALERTE: %s", message)
	if webhookURL == "" {
		return
	}
	if err := sendWebhook(webhookURL, message); err != nil {
		log.Printf("Erreur envoi webhook: %v", err)
	}
}

func sendWebhook(webhookURL, message string) error {
	body, err := json.Marshal(map[string]string{"text": message})
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("statut HTTP %d", resp.StatusCode)
	}
	return nil
}
