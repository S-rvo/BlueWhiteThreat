// apps/classifier/internal/db/mongo.go
package db

import (
	"context"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func getEnvOrDefault(key, defaultValue string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultValue
}

// Connect opens a MongoDB connection using the same MONGO_* environment
// variables as apps/scraper, so both services can share one .env in compose.
func Connect(ctx context.Context) (*mongo.Client, error) {
	user := getEnvOrDefault("MONGO_INITDB_ROOT_USERNAME", "root")
	pass := getEnvOrDefault("MONGO_INITDB_ROOT_PASSWORD", "root")
	host := getEnvOrDefault("MONGO_HOST", "localhost")
	port := getEnvOrDefault("MONGO_PORT", "27017")

	uri := "mongodb://" + user + ":" + pass + "@" + host + ":" + port + "/"
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return mongo.Connect(ctx, options.Client().ApplyURI(uri))
}

// ArticleContent is the minimal projection needed for scoring: the article's
// Mongo _id and its scraped text.
type ArticleContent struct {
	ID      interface{} `bson:"_id"`
	Content string      `bson:"content"`
}

func articleCollection(client *mongo.Client) *mongo.Collection {
	dbName := getEnvOrDefault("MONGO_DB", "BlueWhiteThreat")
	return client.Database(dbName).Collection("article")
}

// FetchAllContent returns every article's id and content, regardless of
// relevance status — used by cmd/bootstrap to build a labeling candidate set.
func FetchAllContent(ctx context.Context, client *mongo.Client) ([]ArticleContent, error) {
	cursor, err := articleCollection(client).Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []ArticleContent
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

// FetchUnclassified returns articles that have not yet been scored, i.e. that
// have no "relevance" field at all — distinct from articles scored as
// not-relevant, which must not be re-processed on every tick.
func FetchUnclassified(ctx context.Context, client *mongo.Client) ([]ArticleContent, error) {
	filter := bson.M{"relevance": bson.M{"$exists": false}}
	cursor, err := articleCollection(client).Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []ArticleContent
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

// Relevance mirrors the "relevance" subdocument written onto each article.
type Relevance struct {
	Relevant     bool    `bson:"relevant"`
	Score        float64 `bson:"score"`
	ModelVersion string  `bson:"model_version"`
}

// UpdateRelevance sets the "relevance" field on the article identified by id.
func UpdateRelevance(ctx context.Context, client *mongo.Client, id interface{}, r Relevance) error {
	_, err := articleCollection(client).UpdateOne(
		ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{"relevance": r}},
	)
	return err
}
