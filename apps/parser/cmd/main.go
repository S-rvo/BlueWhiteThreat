package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"

	"github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/db"
	"github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/extractor"
	"github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/pipeline"
)

var defaultIntervalMin = 10

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("Aucun .env trouvé ou problème de lecture.")
	}
	if i, err := strconv.Atoi(os.Getenv("PARSER_INTERVAL_MIN")); err == nil && i > 0 {
		defaultIntervalMin = i
	}

	if err := db.InitNeo4jDriver(os.Getenv("NEO4J_URI"), os.Getenv("NEO4J_USER"), os.Getenv("NEO4J_PASSWORD")); err != nil {
		log.Fatalf("Error initializing Neo4j: %v", err)
	}
	defer db.CloseNeo4j()

	ollamaHost := getEnvOrDefault("OLLAMA_HOST", "http://localhost:11434")
	ollamaModel := getEnvOrDefault("OLLAMA_MODEL", "qwen2.5:14b-instruct")
	primary := extractor.NewOllamaExtractor(ollamaHost, ollamaModel)
	fallback := extractor.NewClaudeExtractor()

	log.Printf("Lancement du parser (interval: %d min)", defaultIntervalMin)
	runOnce(primary, fallback)

	ticker := time.NewTicker(time.Duration(defaultIntervalMin) * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		runOnce(primary, fallback)
	}
}

func getEnvOrDefault(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func runOnce(primary, fallback extractor.Extractor) {
	ctx := context.Background()
	client, err := db.ConnectMongo(ctx)
	if err != nil {
		log.Printf("Erreur connexion MongoDB: %v", err)
		return
	}
	defer client.Disconnect(ctx)

	articles, err := db.FetchUnparsedArticles(ctx, client, 50)
	if err != nil {
		log.Printf("Erreur lecture articles: %v", err)
		return
	}
	log.Printf("Parser: %d articles à traiter", len(articles))

	for _, a := range articles {
		ents, err := pipeline.ExtractWithFallback(ctx, primary, fallback, a.Content)
		if err != nil {
			log.Printf("Erreur extraction article %v: %v", a.ID, err)
			continue
		}
		if err := db.SaveEntities(ctx, client, a.ID, ents); err != nil {
			log.Printf("Erreur écriture Mongo article %v: %v", a.ID, err)
			continue
		}
		if err := db.SaveEntityGraph(ctx, idToString(a.ID), ents); err != nil {
			log.Printf("Erreur écriture Neo4j article %v: %v", a.ID, err)
		}
	}
}

func idToString(id any) string {
	return fmt.Sprintf("%v", id)
}
