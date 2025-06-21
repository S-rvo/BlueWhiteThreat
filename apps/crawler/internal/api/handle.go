package api

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/S-rvo/BlueWhiteThreat/internal/db"
)

// HealthCheckHandler vérifie si l'API est up
func HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
		log.Printf("Error encoding health check response: %v", err)
	}
}

// GetGraphHandler retourne toutes les relations principales (URLs nettoyées) du graphe Neo4j
func GetGraphHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		graph, err := db.FetchGraphRelations()
		if err != nil {
			log.Printf("Error fetching graph relations from Neo4j: %v", err)
			http.Error(w, `{"error":"Neo4j error fetching graph relations"}`, http.StatusInternalServerError)
			return
		}

		if err := json.NewEncoder(w).Encode(graph); err != nil {
			log.Printf("Error encoding graph response: %v", err)
		}
	}
}

// GetAllURLsHandler retourne toutes les URLs principales (nettoyées)
func GetAllURLsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		urls, err := db.FetchAllURLs()
		if err != nil {
			log.Printf("Error fetching URLs from Neo4j: %v", err)
			http.Error(w, `{"error":"Neo4j error fetching URLs"}`, http.StatusInternalServerError)
			return
		}

		if err := json.NewEncoder(w).Encode(urls); err != nil {
			log.Printf("Error encoding URLs response: %v", err)
		}
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
