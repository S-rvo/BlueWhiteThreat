package db

import (
	"context"
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

// FetchUnparsedArticles renvoie les articles n'ayant pas encore de champ "entities".
func FetchUnparsedArticles(ctx context.Context, client *mongo.Client, limit int64) ([]UnparsedArticle, error) {
	filter := bson.M{"entities": bson.M{"$exists": false}}
	cur, err := articleCollection(client).Find(ctx, filter, options.Find().SetLimit(limit))
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
// et éviter de le retraiter en boucle.
func SaveEntities(ctx context.Context, client *mongo.Client, articleID any, ents entities.Entities) error {
	_, err := articleCollection(client).UpdateOne(ctx,
		bson.M{"_id": articleID, "entities": bson.M{"$exists": false}},
		bson.M{
			"$set":         bson.M{"entities": ents},
			"$currentDate": bson.M{"graph_projection_queued_at": true},
		},
	)
	return err
}

// FetchPendingGraphArticles includes existing extractions without a projection
// marker. Oldest queued articles run first; failures move to the back so a
// broken article does not block the batch and retries get their turn.
func FetchPendingGraphArticles(ctx context.Context, client *mongo.Client, limit int64) ([]PendingGraphArticle, error) {
	filter := bson.M{
		"entities":           bson.M{"$exists": true},
		"graph_projected_at": bson.M{"$exists": false},
	}
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
