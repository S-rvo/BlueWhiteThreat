package db

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoDB struct {
	Client     *mongo.Client
	Database   *mongo.Database
	Collection *mongo.Collection
}

// InitMongoDB initialise la connexion à MongoDB
func InitMongoDB() (*MongoDB, error) {
	collectionName := os.Getenv("MONGO_COLLECTION_NAME")
	if collectionName == "" {
		collectionName = "default"
	}
	dbName := os.Getenv("MONGO_DBNAME")
	if dbName == "" {
		dbName = "dbname"
		log.Println(dbName)
	}
	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		uri = "mongodb://mongodb_service:27017"
		log.Println(uri)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientOpts := options.Client().ApplyURI(uri)
	client, err := mongo.Connect(ctx, clientOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	db := client.Database(dbName)
	collection := db.Collection(collectionName)

	log.Println("Connected to MongoDB")

	return &MongoDB{
		Client:     client,
		Database:   db,
		Collection: collection,
	}, nil
}

// Insert insère un document dans la collection
func (m *MongoDB) Insert(document interface{}) (*mongo.InsertOneResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := m.Collection.InsertOne(ctx, document)
	if err != nil {
		return nil, fmt.Errorf("failed to insert document: %w", err)
	}
	return result, nil
}

// Close ferme la connexion MongoDB
func (m *MongoDB) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return m.Client.Disconnect(ctx)
}

// Méthode pour créer un index unique sur url_crawled dans MongoDB
func (m *MongoDB) EnsureUniqueIndex() error {
	indexModel := mongo.IndexModel{
		Keys:    bson.M{"url_crawled": 1},
		Options: options.Index().SetUnique(true),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := m.Collection.Indexes().CreateOne(ctx, indexModel)
	return err
}
