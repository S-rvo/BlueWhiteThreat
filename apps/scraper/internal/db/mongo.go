package db

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/utils"
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
