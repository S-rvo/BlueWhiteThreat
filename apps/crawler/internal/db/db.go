package db

import (
    "context"
    "log"
    "os"
    "time"

    "github.com/S-rvo/BlueWhiteThreat/internal/models"
    "github.com/joho/godotenv"
    "go.mongodb.org/mongo-driver/bson"
    "go.mongodb.org/mongo-driver/mongo"
    "go.mongodb.org/mongo-driver/mongo/options"
)

// Variables globales exportées
var (
    MongoClient     *mongo.Client
    Database        *mongo.Database
    URLsCollection  *mongo.Collection
    PagesCollection *mongo.Collection
    ErrorsCollection *mongo.Collection
)

// LoadEnv charge les variables d'environnement
func LoadEnv() error {
    err := godotenv.Load()
    if err != nil {
        log.Printf("Warning: .env file not found: %v", err)
    }
    return nil
}

// InitDB initialise la connexion à la base de données
func InitDB() error {
    // Charger les variables d'environnement
    LoadEnv()

    // Récupérer les paramètres de connexion
    mongoURI := os.Getenv("MONGO_URI")
    if mongoURI == "" {
        mongoURI = "mongodb://localhost:27017"
    }

    dbName := os.Getenv("CRAWLER_DB_NAME")
    if dbName == "" {
        dbName = "torcrawler"
    }

    // Connexion à MongoDB
    clientOptions := options.Client().ApplyURI(mongoURI)
    client, err := mongo.Connect(context.Background(), clientOptions)
    if err != nil {
        return err
    }

    // Vérifier la connexion
    err = client.Ping(context.Background(), nil)
    if err != nil {
        return err
    }

    // Initialiser les variables globales
    MongoClient = client
    Database = client.Database(dbName)
    URLsCollection = Database.Collection("urls")
    PagesCollection = Database.Collection("pages")
    ErrorsCollection = Database.Collection("errors")

    // Créer des index
    indexURL := mongo.IndexModel{
        Keys:    bson.M{"url": 1},
        Options: options.Index().SetUnique(true),
    }

    _, err = URLsCollection.Indexes().CreateOne(context.Background(), indexURL)
    if err != nil {
        log.Printf("Warning: Failed to create URL index: %v", err)
    }

    _, err = PagesCollection.Indexes().CreateOne(context.Background(), indexURL)
    if err != nil {
        log.Printf("Warning: Failed to create page URL index: %v", err)
    }

    log.Printf("Connected to MongoDB: %s, database: %s", mongoURI, dbName)
    return nil
}

// CloseDB ferme la connexion à la base de données
func CloseDB() {
    if MongoClient != nil {
        if err := MongoClient.Disconnect(context.Background()); err != nil {
            log.Printf("Error disconnecting from MongoDB: %v", err)
        }
    }
}

// InsertURL insère une URL dans la collection des URLs
func InsertURL(url string) error {
    entry := models.URLEntry{
        URL:     url,
        Visited: false,
        AddedAt: time.Now(),
    }

    _, err := URLsCollection.InsertOne(context.Background(), entry)
    if err != nil {
        // Ignorer les erreurs de duplication d'index
        if mongo.IsDuplicateKeyError(err) {
            return nil
        }
        return err
    }
    return nil
}
