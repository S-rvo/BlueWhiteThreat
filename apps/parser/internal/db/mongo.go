package db

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/entities"
)

func getEnvOrDefault(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func ConnectMongo(ctx context.Context) (*mongo.Client, error) {
	if uri := os.Getenv("MONGODB_URI"); uri != "" {
		return mongo.Connect(ctx, options.Client().ApplyURI(uri).SetServerSelectionTimeout(10*time.Second))
	}
	user := getEnvOrDefault("MONGO_INITDB_ROOT_USERNAME", "root")
	pass := getEnvOrDefault("MONGO_INITDB_ROOT_PASSWORD", "root")
	host := getEnvOrDefault("MONGO_HOST", "localhost")
	port := getEnvOrDefault("MONGO_PORT", "27017")
	uri := "mongodb://" + user + ":" + pass + "@" + host + ":" + port + "/"

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return mongo.Connect(ctx, options.Client().ApplyURI(uri))
}

type UnparsedArticle struct {
	ID      any    `bson:"_id"`
	Content string `bson:"content"`
}

type PendingGraphArticle struct {
	ID       any               `bson:"_id"`
	Entities entities.Entities `bson:"entities"`
}

func articleCollection(client *mongo.Client) *mongo.Collection {
	return client.Database("BlueWhiteThreat").Collection("article")
}

// A positive verdict flagged as uncertain stays out of automatic extraction/projection.
func eligibleArticles() bson.M {
	return bson.M{"relevance.relevant": true, "relevance.needs_review": bson.M{"$ne": true}}
}

// Champs de suivi des échecs d'extraction. Le scraper les efface quand le
// contenu d'une page change : un nouveau texte repart sans historique.
const (
	extractionFailuresField  = "extraction_failures"
	extractionRetryAtField   = "extraction_retry_at"
	extractionLastErrorField = "extraction_last_error"
)

// Backoff exponentiel des extractions en échec : 10 min, 20 min, … plafonné à
// 24 h. Il n'y a pas de quarantaine définitive : une panne longue d'Ollama ne
// doit pas condamner les articles touchés.
const (
	extractionBaseBackoff = 10 * time.Minute
	extractionMaxBackoff  = 24 * time.Hour
)

// ExtractionBackoff renvoie le délai avant la tentative suivant le n-ième échec.
func ExtractionBackoff(failures int) time.Duration {
	d := extractionBaseBackoff
	for i := 1; i < failures; i++ {
		d *= 2
		if d >= extractionMaxBackoff {
			return extractionMaxBackoff
		}
	}
	return d
}

// EnsureIndexes crée les index utilisés par les files du parser. Idempotent.
func EnsureIndexes(ctx context.Context, client *mongo.Client) error {
	_, err := articleCollection(client).Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "relevance.relevant", Value: 1}, {Key: "_id", Value: 1}},
			Options: options.Index().SetName("parser_extraction_queue"),
		},
		{
			Keys:    bson.D{{Key: "relevance.relevant", Value: 1}, {Key: "graph_projection_queued_at", Value: 1}, {Key: "_id", Value: 1}},
			Options: options.Index().SetName("parser_projection_queue"),
		},
		{
			Keys: bson.D{{Key: "graph_projected_at", Value: 1}},
			Options: options.Index().SetName("parser_graph_projected").
				SetPartialFilterExpression(bson.M{"graph_projected_at": bson.M{"$exists": true}}),
		},
	})
	if err != nil {
		return fmt.Errorf("db: parser indexes: %w", err)
	}
	return nil
}

// FetchUnparsedArticles renvoie les articles jugés pertinents et sans extraction,
// hors articles en attente de backoff après un échec.
func FetchUnparsedArticles(ctx context.Context, client *mongo.Client, limit int64) ([]UnparsedArticle, error) {
	filter := eligibleArticles()
	filter["entities"] = bson.M{"$exists": false}
	filter["$or"] = bson.A{
		bson.M{extractionRetryAtField: bson.M{"$exists": false}},
		bson.M{extractionRetryAtField: bson.M{"$lte": time.Now()}},
	}
	cur, err := articleCollection(client).Find(ctx, filter, options.Find().SetLimit(limit).SetProjection(bson.M{"content": 1}).SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []UnparsedArticle
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SaveEntities écrit le résultat d'extraction sur le document article correspondant.
// Un résultat vide est aussi écrit (tableaux vides) pour marquer l'article comme traité
// et éviter de le retraiter en boucle. L'article repart en projection même s'il
// avait déjà été projeté : le graphe doit refléter cette nouvelle extraction.
func SaveEntities(ctx context.Context, client *mongo.Client, article UnparsedArticle, ents entities.Entities) error {
	filter := eligibleArticles()
	filter["_id"], filter["content"], filter["entities"] = article.ID, article.Content, bson.M{"$exists": false}
	_, err := articleCollection(client).UpdateOne(ctx, filter,
		bson.M{
			"$set":         bson.M{"entities": ents},
			"$currentDate": bson.M{"graph_projection_queued_at": true},
			"$unset": bson.M{
				"graph_projected_at":     "",
				extractionFailuresField:  "",
				extractionRetryAtField:   "",
				extractionLastErrorField: "",
			},
		},
	)
	return err
}

// RecordExtractionFailure compte un échec et repousse la prochaine tentative,
// pour qu'un article qui échoue toujours ne bloque pas la tête de file. Sans
// effet si le contenu a changé entre-temps.
func RecordExtractionFailure(ctx context.Context, client *mongo.Client, article UnparsedArticle, cause error) (int, error) {
	filter := bson.M{"_id": article.ID, "content": article.Content, "entities": bson.M{"$exists": false}}
	message := cause.Error()
	if len(message) > 500 {
		message = message[:500]
	}
	var doc struct {
		Failures int `bson:"extraction_failures"`
	}
	err := articleCollection(client).FindOneAndUpdate(ctx, filter,
		bson.M{"$inc": bson.M{extractionFailuresField: 1}, "$set": bson.M{extractionLastErrorField: message}},
		options.FindOneAndUpdate().SetReturnDocument(options.After).SetProjection(bson.M{extractionFailuresField: 1}),
	).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	retryAt := time.Now().Add(ExtractionBackoff(doc.Failures))
	_, err = articleCollection(client).UpdateOne(ctx, filter, bson.M{"$set": bson.M{extractionRetryAtField: retryAt}})
	return doc.Failures, err
}

// FetchPendingGraphArticles includes existing extractions without a projection
// marker. Oldest queued articles run first; failures move to the back so a
// broken article does not block the batch and retries get their turn.
func FetchPendingGraphArticles(ctx context.Context, client *mongo.Client, limit int64) ([]PendingGraphArticle, error) {
	filter := eligibleArticles()
	filter["entities"] = bson.M{"$exists": true}
	filter["graph_projected_at"] = bson.M{"$exists": false}
	opts := options.Find().
		SetLimit(limit).
		SetProjection(bson.M{"entities": 1}).
		SetSort(bson.D{{Key: "graph_projection_queued_at", Value: 1}, {Key: "_id", Value: 1}})
	cur, err := articleCollection(client).Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []PendingGraphArticle
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func MarkGraphProjected(ctx context.Context, client *mongo.Client, articleID any) error {
	_, err := articleCollection(client).UpdateOne(ctx, bson.M{"_id": articleID}, bson.M{
		"$currentDate": bson.M{"graph_projected_at": true},
		"$unset":       bson.M{"graph_projection_queued_at": ""},
	})
	return err
}

func RequeueGraphProjection(ctx context.Context, client *mongo.Client, articleID any) error {
	filter := bson.M{"_id": articleID, "graph_projected_at": bson.M{"$exists": false}}
	_, err := articleCollection(client).UpdateOne(ctx, filter, bson.M{
		"$currentDate": bson.M{"graph_projection_queued_at": true},
	})
	return err
}

// FetchRetractableArticles renvoie les articles encore présents dans le graphe
// alors qu'ils ne sont plus éligibles : reclassés non pertinents, passés en
// revue, ou dont le contenu a changé et attend une nouvelle classification.
func FetchRetractableArticles(ctx context.Context, client *mongo.Client, limit int64) ([]any, error) {
	filter := bson.M{
		"graph_projected_at": bson.M{"$exists": true},
		"$nor":               bson.A{eligibleArticles()},
	}
	cur, err := articleCollection(client).Find(ctx, filter, options.Find().SetLimit(limit).SetProjection(bson.M{"_id": 1}).SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var docs []struct {
		ID any `bson:"_id"`
	}
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	ids := make([]any, 0, len(docs))
	for _, d := range docs {
		ids = append(ids, d.ID)
	}
	return ids, nil
}

// MarkGraphRetracted enregistre que l'article n'est plus dans le graphe. S'il
// est redevenu éligible entre-temps, il sera simplement reprojeté.
func MarkGraphRetracted(ctx context.Context, client *mongo.Client, articleID any) error {
	_, err := articleCollection(client).UpdateOne(ctx, bson.M{"_id": articleID}, bson.M{
		"$unset": bson.M{"graph_projected_at": "", "graph_projection_queued_at": ""},
	})
	return err
}
