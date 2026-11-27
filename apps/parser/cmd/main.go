package main

import (
	"context"
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

// Après autant d'échecs consécutifs, le cycle d'extraction s'arrête : c'est
// plus probablement une panne du modèle qu'une série d'articles pathologiques,
// et il ne faut pas repousser toute la file à cause d'elle.
const maxConsecutiveExtractionFailures = 3

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
	schemaCtx, cancelSchema := context.WithTimeout(context.Background(), 2*time.Minute)
	if err := db.EnsureGraphSchema(schemaCtx); err != nil {
		// Le parser reste utile sans contraintes ; l'erreur doit être corrigée
		// (doublons hérités) mais ne doit pas bloquer l'extraction.
		log.Printf("Schéma Neo4j incomplet: %v", err)
	}
	cancelSchema()

	ollamaHost := getEnvOrDefault("OLLAMA_HOST", "http://localhost:11434")
	ollamaModel := getEnvOrDefault("OLLAMA_MODEL", "qwen2.5:14b-instruct")
	primary := extractor.NewOllamaExtractor(ollamaHost, ollamaModel)
	var fallback extractor.Extractor
	switch os.Getenv("PARSER_ENABLE_REMOTE_FALLBACK") {
	case "", "false":
	case "true":
		if os.Getenv("ANTHROPIC_API_KEY") == "" {
			log.Fatal("PARSER_ENABLE_REMOTE_FALLBACK=true requires ANTHROPIC_API_KEY")
		}
		fallback = extractor.NewClaudeExtractor()
	default:
		log.Fatal("PARSER_ENABLE_REMOTE_FALLBACK must be true or false")
	}

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

	indexCtx, cancelIndex := context.WithTimeout(ctx, 30*time.Second)
	if err := db.EnsureIndexes(indexCtx, client); err != nil {
		log.Printf("Index MongoDB du parser: %v", err)
	}
	cancelIndex()

	extractPending(ctx, client, primary, fallback)
	retractStale(ctx, client, db.RetractEntityGraph)
	projectPending(ctx, client, db.ReplaceEntityGraph)
}

func extractPending(ctx context.Context, client *mongo.Client, primary, fallback extractor.Extractor) {
	readCtx, cancelRead := context.WithTimeout(ctx, 10*time.Second)
	articles, err := db.FetchUnparsedArticles(readCtx, client, batchSize)
	cancelRead()
	if err != nil {
		log.Printf("Erreur lecture articles: %v", err)
		return
	}
	log.Printf("Parser: %d articles à traiter", len(articles))

	consecutiveFailures := 0
	for _, a := range articles {
		ents, err := pipeline.ExtractWithFallback(ctx, primary, fallback, a.Content)
		if err != nil {
			failCtx, cancelFail := context.WithTimeout(ctx, 10*time.Second)
			failures, recordErr := db.RecordExtractionFailure(failCtx, client, a, err)
			cancelFail()
			log.Printf("Erreur extraction article %v (échec n°%d, prochain essai dans %s): %v",
				a.ID, failures, db.ExtractionBackoff(failures), err)
			if recordErr != nil {
				log.Printf("Erreur enregistrement de l'échec article %v: %v", a.ID, recordErr)
			}
			consecutiveFailures++
			if consecutiveFailures >= maxConsecutiveExtractionFailures {
				log.Printf("Parser: %d échecs consécutifs, cycle d'extraction interrompu", consecutiveFailures)
				return
			}
			continue
		}
		consecutiveFailures = 0
		saveCtx, cancelSave := context.WithTimeout(ctx, 10*time.Second)
		err = db.SaveEntities(saveCtx, client, a, ents)
		cancelSave()
		if err != nil {
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
		// Toujours écrire, même vide : une extraction vide doit retirer les
		// entités d'une version précédente de l'article.
		graphCtx, cancel := context.WithTimeout(cycleCtx, 30*time.Second)
		err := write(graphCtx, db.ArticleKey(article.ID), article.Entities)
		cancel()
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

// retractStale retire du graphe les articles qui ne sont plus éligibles. En cas
// d'échec, l'article reste marqué projeté et sera repris au cycle suivant.
func retractStale(ctx context.Context, client *mongo.Client, retract func(context.Context, string) error) {
	cycleCtx, cancelCycle := context.WithTimeout(ctx, 2*time.Minute)
	defer cancelCycle()
	ids, err := db.FetchRetractableArticles(cycleCtx, client, batchSize)
	if err != nil {
		log.Printf("Erreur lecture des rétractations en attente: %v", err)
		return
	}
	for _, id := range ids {
		if cycleCtx.Err() != nil {
			return
		}
		graphCtx, cancel := context.WithTimeout(cycleCtx, 30*time.Second)
		err := retract(graphCtx, db.ArticleKey(id))
		cancel()
		if err != nil {
			log.Printf("Erreur rétractation Neo4j article %v: %v", id, err)
			continue
		}
		statusCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if err := db.MarkGraphRetracted(statusCtx, client, id); err != nil {
			log.Printf("Erreur validation rétractation article %v: %v", id, err)
		}
		cancel()
	}
}
