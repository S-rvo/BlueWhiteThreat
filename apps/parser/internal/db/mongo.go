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

// FetchUnparsedArticles renvoie les articles n'ayant pas encore de champ "entities".
func FetchUnparsedArticles(ctx context.Context, client *mongo.Client, limit int64) ([]UnparsedArticle, error) {
	coll := client.Database("BlueWhiteThreat").Collection("article")
	filter := bson.M{"entities": bson.M{"$exists": false}}
	cur, err := coll.Find(ctx, filter, options.Find().SetLimit(limit))
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
	coll := client.Database("BlueWhiteThreat").Collection("article")
	_, err := coll.UpdateOne(ctx,
		bson.M{"_id": articleID},
		bson.M{"$set": bson.M{"entities": ents}},
	)
	return err
}
