package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/db"
	"github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/entities"
	"github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/extractor"
	"github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/pipeline"
)

var defaultIntervalMin = 10

const batchSize = 50

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

	extractPending(ctx, client, primary, fallback)
	projectPending(ctx, client, db.SaveEntityGraph)
}

func extractPending(ctx context.Context, client *mongo.Client, primary, fallback extractor.Extractor) {
	articles, err := db.FetchUnparsedArticles(ctx, client, batchSize)
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
		}
	}
}

func projectPending(ctx context.Context, client *mongo.Client, write func(context.Context, string, entities.Entities) error) {
	cycleCtx, cancelCycle := context.WithTimeout(ctx, 2*time.Minute)
	defer cancelCycle()
	articles, err := db.FetchPendingGraphArticles(cycleCtx, client, batchSize)
	if err != nil {
		log.Printf("Erreur lecture des projections en attente: %v", err)
		return
	}
	for _, article := range articles {
		if cycleCtx.Err() != nil {
			return
		}
		var err error
		if !article.Entities.IsEmpty() {
			graphCtx, cancel := context.WithTimeout(cycleCtx, 30*time.Second)
			err = write(graphCtx, fmt.Sprint(article.ID), article.Entities)
			cancel()
		}
		statusCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if err != nil {
			log.Printf("Erreur écriture Neo4j article %v: %v", article.ID, err)
			if err := db.RequeueGraphProjection(statusCtx, client, article.ID); err != nil {
				log.Printf("Erreur remise en file de la projection Neo4j article %v: %v", article.ID, err)
			}
		} else if err := db.MarkGraphProjected(statusCtx, client, article.ID); err != nil {
			log.Printf("Erreur validation projection article %v: %v", article.ID, err)
		}
		cancel()
	}
}
