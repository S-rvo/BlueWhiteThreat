package api

import (
	"encoding/json"
	"net/http"

	"github.com/S-rvo/BlueWhiteThreat/internal/db"
)

// HealthCheckHandler vérifie si l'API est up
func HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// GetGraphHandler retourne toutes les relations URL -> URL du graphe Neo4j
func GetGraphHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		graph, err := db.FetchGraphRelations()
		if err != nil {
			http.Error(w, `{"error":"Neo4j error"}`, http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(graph)
	}
}

// GetAllURLsHandler retourne toutes les URLs du graphe
func GetAllURLsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		urls, err := db.FetchAllURLs()
		if err != nil {
			http.Error(w, `{"error":"Neo4j error fetching URLs"}`, http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(urls)
	}
}

// GetCrawlerStatsHandler retourne les statistiques du crawler depuis Redis
func GetCrawlerStatsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		stats, err := db.FetchCrawlerStats()
		if err != nil {
			http.Error(w, `{"error":"Redis error fetching stats"}`, http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(stats)
	}
}
