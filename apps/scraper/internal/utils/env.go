package utils

import "os"

// GetEnvOrDefault renvoie la valeur d'une variable d'env ou une valeur par défaut si elle n'existe pas.
func GetEnvOrDefault(key string, defaultValue string) string {
    if val, ok := os.LookupEnv(key); ok {
        return val
    }
    return defaultValue
}