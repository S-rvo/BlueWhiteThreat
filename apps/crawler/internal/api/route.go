package api

import (
	"net/http"
)

// NewRouter configure et retourne toutes les routes de l'API
func NewRouter() *http.ServeMux {
	mux := http.NewServeMux()

	// Routes API
	mux.HandleFunc("/health", HealthCheckHandler)
	mux.HandleFunc("/graph", GetGraphHandler())
	mux.HandleFunc("/urls", GetAllURLsHandler())
	mux.HandleFunc("/stats", GetCrawlerStatsHandler())

	return mux
}
