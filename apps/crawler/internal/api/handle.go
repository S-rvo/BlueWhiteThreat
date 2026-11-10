package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/crawler/internal/db"
)

// HealthCheckHandler vérifie si l'API est up
func HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
		log.Printf("Error encoding health check response: %v", err)
	}
}

// GetCrawlerStatsHandler retourne les statistiques du crawler depuis Redis
func GetCrawlerStatsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		stats, err := db.FetchCrawlerStats()
		if err != nil {
			log.Printf("Error fetching crawler stats from Redis: %v", err)
			http.Error(w, `{"error":"Redis error fetching crawler stats"}`, http.StatusInternalServerError)
			return
		}

		if err := json.NewEncoder(w).Encode(stats); err != nil {
			log.Printf("Error encoding stats response: %v", err)
		}
	}
}

func ReadinessHandler(w http.ResponseWriter, r *http.Request) {
	if err := db.Ready(); err != nil {
		http.Error(w, "dependencies unavailable", http.StatusServiceUnavailable)
		return
	}
	HealthCheckHandler(w, r)
}

func HostStatsHandler(w http.ResponseWriter, r *http.Request) {
	host := strings.ToLower(r.URL.Query().Get("host"))
	if host == "" || len(host) > 300 || strings.ContainsAny(host, "/\\ \n\r") {
		http.Error(w, "invalid host", 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if db.RedisClient == nil {
		http.Error(w, "unavailable", 503)
		return
	}
	stats, err := db.RedisClient.HGetAll(ctx, "crawler:host_stats:"+host).Result()
	if err != nil {
		http.Error(w, "unavailable", 503)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		log.Printf("host stats: %v", err)
	}
}
