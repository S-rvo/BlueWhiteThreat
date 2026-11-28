package db

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/utils"
	"github.com/S-rvo/BlueWhiteThreat/pkg/contract"
)

// Connexion MongoDB centralisée
func ConnectMongo() (*mongo.Client, error) {
	user := utils.GetEnvOrDefault("MONGO_INITDB_ROOT_USERNAME", "root")
	pass := utils.GetEnvOrDefault("MONGO_INITDB_ROOT_PASSWORD", "root")
	host := utils.GetEnvOrDefault("MONGO_HOST", "localhost")
	port := utils.GetEnvOrDefault("MONGO_PORT", "27017")

	uri := "mongodb://" + user + ":" + pass + "@" + host + ":" + port + "/"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	return client, err
}

// EnsureArticleIndexes garantit l'unicité de l'URL, clé de l'upsert du scraper :
// sans elle, deux workers concurrents peuvent insérer deux fois la même page.
// Idempotent ; échoue si la collection contient déjà des doublons.
func EnsureArticleIndexes(ctx context.Context, client *mongo.Client) error {
	_, err := client.Database(contract.Database).Collection(contract.ArticleCollection).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: contract.FieldURL, Value: 1}},
		// Partiel : seuls les documents portant une URL sont concernés ; un
		// document sans url (fixture, import) ne doit pas entrer en collision
		// avec les autres sur la valeur null.
		Options: options.Index().SetName("scraper_url_unique").SetUnique(true).
			SetPartialFilterExpression(bson.M{contract.FieldURL: bson.M{"$type": "string"}}),
	})
	return err
}
