package utils

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"
)

// GetEnvOrDefault renvoie la valeur d'une variable d'env ou une valeur par défaut si elle n'existe pas.
func GetEnvOrDefault(key string, defaultValue string) string {
    if val, ok := os.LookupEnv(key); ok {
        return val
    }
    return defaultValue
}

// Un client HTTP avec timeout, global/lazy init si besoin :
var HttpClient = &http.Client{
    Timeout: 10 * time.Second,
}

// Helper pour une requête GET sécurisée
func SafeGetURL(u string) (*http.Response, error) {
    parsed, err := url.Parse(u)
    if err != nil {
        return nil, fmt.Errorf("url invalide: %w", err)
    }
    // Si tu veux limiter le host (optionnel):
    if parsed.Host != "raw.githubusercontent.com" && parsed.Host != "api.github.com" {
        return nil, fmt.Errorf("host non autorisé: %s", parsed.Host)
    }
    return HttpClient.Get(parsed.String())
}