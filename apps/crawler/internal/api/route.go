package api

import (
	"net/http"
)

// NewRouter configure et retourne toutes les routes de l'API
func NewRouter() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", HealthCheckHandler)
	mux.HandleFunc("/stats", GetCrawlerStatsHandler())

	return CORS(mux)
}
