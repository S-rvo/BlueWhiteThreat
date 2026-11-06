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
	// seuil franchi. Persisté dans Redis (db.TryMarkAlertSent) : un
	// redémarrage du conteneur ne réémet pas une alerte déjà envoyée.
	alertCooldown = 6 * time.Hour
)

// condition identifie un type d'alerte, utilisé comme clé de cooldown.
type condition string

const (
	conditionQueueCapacity condition = "queue_capacity"
	conditionFreshness     condition = "freshness"
)

// alert associe une condition déclenchée à son message, pour séparer
// l'évaluation des seuils (pure, testable) de l'envoi (effets de bord).
type alert struct {
	condition condition
	message   string
}

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

	trySend := func(c condition) (bool, error) {
		return db.TryMarkAlertSent(string(c), alertCooldown)
	}

	checkOnce := func() {
		if err := checkThresholds(webhookURL, trySend); err != nil {
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

// checkThresholds lit les métriques courantes depuis Redis, détermine quelles
// alertes doivent se déclencher (evaluateThresholds, pure), puis les émet
// (maybeAlert, avec cooldown via trySend).
func checkThresholds(webhookURL string, trySend func(condition) (bool, error)) error {
	ratio, err := db.QueueCapacityRatio()
	if err != nil {
		return fmt.Errorf("lecture ratio de capacité: %w", err)
	}

	stats, err := db.FetchCrawlerStats()
	if err != nil {
		return fmt.Errorf("lecture des stats: %w", err)
	}
	var oldestAge time.Duration
	hasAge := false
	if raw, ok := stats["oldest_crawled_age_seconds"]; ok {
		ageSeconds, err := strconv.ParseInt(raw, 10, 64)
		if err == nil {
			oldestAge = time.Duration(ageSeconds) * time.Second
			hasAge = true
		}
	}

	for _, a := range evaluateThresholds(ratio, oldestAge, hasAge) {
		maybeAlert(webhookURL, trySend, a)
	}
	return nil
}

// evaluateThresholds décide, à partir de métriques déjà lues, quelles
// alertes doivent se déclencher. Aucun appel réseau/Redis ici : testable
// sans dépendance externe.
func evaluateThresholds(queueRatio float64, oldestAge time.Duration, hasAge bool) []alert {
	var alerts []alert

	if queueRatio >= queueCapacityWarnRatio {
		alerts = append(alerts, alert{
			condition: conditionQueueCapacity,
			message:   fmt.Sprintf("Crawler: file d'attente à %.0f%% de sa capacité (seuil: %.0f%%)", queueRatio*100, queueCapacityWarnRatio*100),
		})
	}

	if hasAge && oldestAge > staleAgeWarnThreshold {
		alerts = append(alerts, alert{
			condition: conditionFreshness,
			message:   fmt.Sprintf("Crawler: l'URL la plus ancienne non re-crawlée date de %s (seuil: %s) -- le sweep de fraîcheur semble bloqué", oldestAge.Round(time.Hour), staleAgeWarnThreshold),
		})
	}

	return alerts
}

// maybeAlert envoie l'alerte sauf si trySend indique qu'elle est encore en
// cooldown (déjà envoyée récemment pour cette condition).
func maybeAlert(webhookURL string, trySend func(condition) (bool, error), a alert) {
	shouldSend, err := trySend(a.condition)
	if err != nil {
		log.Printf("Erreur vérification du cooldown pour %s: %v", a.condition, err)
		return
	}
	if !shouldSend {
		return
	}

	log.Printf("ALERTE: %s", a.message)
	if webhookURL == "" {
		return
	}
	if err := sendWebhook(webhookURL, a.message); err != nil {
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
