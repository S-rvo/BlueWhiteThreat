package api

import (
	"net/http"
)

// NewRouter configure et retourne toutes les routes de l'API
func NewRouter() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", HealthCheckHandler)
	mux.HandleFunc("/graph", GetGraphHandler())
	mux.HandleFunc("/urls", GetAllURLsHandler())
	mux.HandleFunc("/stats", GetCrawlerStatsHandler())

	return CORSMiddleware(mux) // <----- Ici on enveloppe avec CORS
}
