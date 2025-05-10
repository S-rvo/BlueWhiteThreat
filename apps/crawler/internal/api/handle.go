package api

import (
	"encoding/json"
	"net/http"

	"github.com/S-rvo/BlueWhiteThreat/internal/db"
)

// GET /graph
func GetGraphHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		graph, err := db.FetchGraphRelations()
		if err != nil {
			http.Error(w, "Neo4j error", http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(graph)
	}
}
