package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// No raw article content, source URLs, or user data belong in this response.
type summary struct {
	ID         primitive.ObjectID `bson:"_id" json:"id"`
	SourceType string             `bson:"source_type" json:"source_type,omitempty"`
	ScrapedAt  *time.Time         `bson:"scraped_at" json:"scraped_at,omitempty"`
}

type apiStore interface {
	list(context.Context, primitive.ObjectID, int64) ([]summary, error)
	ready(context.Context) error
	metrics(context.Context) (pipelineMetrics, error)
}

func newHandler(store apiStore) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := store.ready(ctx); err != nil {
			http.Error(w, "storage unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
		defer cancel()
		metrics, err := store.metrics(ctx)
		if err != nil {
			http.Error(w, "metrics unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = io.WriteString(w, metrics.render())
	})
	mux.HandleFunc("GET /data", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Add("Vary", "Origin")
		switch origin := r.Header.Get("Origin"); origin {
		case "http://localhost:8080", "http://127.0.0.1:8080":
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Expose-Headers", "X-Next-Cursor")
		}
		limit, cursor, ok := pagination(r.URL.RawQuery)
		if !ok {
			http.Error(w, "invalid limit or cursor", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
		defer cancel()
		rows, err := store.list(ctx, cursor, limit+1)
		if err != nil {
			http.Error(w, "storage unavailable", http.StatusServiceUnavailable)
			return
		}
		if int64(len(rows)) > limit {
			w.Header().Set("X-Next-Cursor", rows[limit-1].ID.Hex())
			rows = rows[:limit]
		}
		if rows == nil {
			rows = []summary{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(rows)
	})
	return mux
}

func pagination(raw string) (int64, primitive.ObjectID, bool) {
	q, err := url.ParseQuery(raw)
	if err != nil {
		return 0, primitive.NilObjectID, false
	}
	limit := int64(25)
	if values, exists := q["limit"]; exists {
		if len(values) != 1 {
			return 0, primitive.NilObjectID, false
		}
		limit, err = strconv.ParseInt(values[0], 10, 64)
		if err != nil || limit < 1 || limit > 100 {
			return 0, primitive.NilObjectID, false
		}
	}
	if values, exists := q["cursor"]; exists {
		if len(values) != 1 {
			return 0, primitive.NilObjectID, false
		}
		id, err := primitive.ObjectIDFromHex(values[0])
		return limit, id, err == nil && !id.IsZero()
	}
	return limit, primitive.NilObjectID, true
}
