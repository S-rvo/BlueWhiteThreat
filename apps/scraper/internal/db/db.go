package internal

import (
    "context"
    "log"
    "os"
    "time"

    "go.mongodb.org/mongo-driver/bson"
    "go.mongodb.org/mongo-driver/mongo"
    "go.mongodb.org/mongo-driver/mongo/options"

    "github.com/S-rvo/BlueWhiteThreat/internal/models"
)


// Collection MongoDB pour le scraper CTI
var (
    CTICollection *mongo.Collection
    mongoClient   *mongo.Client
)

func LoadEnv() error {
    err := godotenv.Load()
    if err != nil {
        log.Printf("Warning: .env file not found or cannot be loaded: %v", err)
    }
    return nil
}

// InitDB initialise la connexion à la base de données et crée les collections nécessaires
func InitDB() error {
    // Charger les variables d'environnement depuis .env
    LoadEnv()

    // Récupération des variables d'environnement
    mongoURI := os.Getenv("MONGO_URI")
    if mongoURI == "" {
        mongoURI = "mongodb://localhost:27017"
    }

    dbName := os.Getenv("SCRAPER_DB_NAME")
    if dbName == "" {
        dbName = "ctiscraper"
    }

    // Connexion à MongoDB
    clientOptions := options.Client().ApplyURI(mongoURI)
    var err error
    mongoClient, err = mongo.Connect(context.Background(), clientOptions)
    if err != nil {
        return err
    }

    // Ping pour vérifier la connexion
    err = mongoClient.Ping(context.Background(), nil)
    if err != nil {
        return err
    }

    // Initialisation des collections
    db := mongoClient.Database(dbName)
    CTICollection = db.Collection("ctidata")

    // Création des index
    _, err = CTICollection.Indexes().CreateOne(
        context.Background(),
        mongo.IndexModel{
            Keys:    bson.D{{Key: "url", Value: 1}},
            Options: options.Index().SetName("url_index").SetUnique(true),
        },
    )
    if err != nil {
        return err
    }

    _, err = CTICollection.Indexes().CreateOne(
        context.Background(),
        mongo.IndexModel{
            Keys:    bson.D{{Key: "title", Value: "text"}, {Key: "content", Value: "text"}},
            Options: options.Index().SetName("text_index"),
        },
    )
    if err != nil {
        return err
    }

    log.Println("MongoDB initialized for CTI scraper")
    return nil
}

// CloseDB ferme la connexion à MongoDB
func CloseDB() {
    if mongoClient != nil {
        mongoClient.Disconnect(context.Background())
    }
}

// InsertCTIData insère des données CTI
func InsertCTIData(data CTIData) error {
    // Vérifier si l'article existe déjà (par URL)
    filter := bson.M{"url": data.URL}
    count, err := CTICollection.CountDocuments(context.Background(), filter)
    if err != nil {
        return err
    }

    if count == 0 {
        // Définir la date de scraping
        data.ScrapedAt = time.Now()

        // Insérer les nouvelles données
        _, err := CTICollection.InsertOne(context.Background(), data)
        return err
    }

    return nil // Données déjà présentes, pas d'erreur
}

// UpdateCTIData met à jour des données CTI
func UpdateCTIData(url string, updatedData CTIData) error {
    filter := bson.M{"url": url}
    update := bson.M{"$set": bson.M{
        "title":        updatedData.Title,
        "content":      updatedData.Content,
        "category":     updatedData.Category,
        "source":       updatedData.Source,
        "published_at": updatedData.PublishedAt,
        "scraped_at":   time.Now(),
    }}
    _, err := CTICollection.UpdateOne(context.Background(), filter, update)
    return err
}

// FindCTIDataByKeyword recherche des données CTI par mot-clé
func FindCTIDataByKeyword(keyword string) ([]CTIData, error) {
    filter := bson.M{
        "$or": []bson.M{
            {"title": bson.M{"$regex": keyword, "$options": "i"}},
            {"content": bson.M{"$regex": keyword, "$options": "i"}},
        },
    }

    cursor, err := CTICollection.Find(context.Background(), filter)
    if err != nil {
        return nil, err
    }
    defer cursor.Close(context.Background())

    var results []CTIData
    if err := cursor.All(context.Background(), &results); err != nil {
        return nil, err
    }

    return results, nil
}

// GetCTIDataBySource récupère toutes les données CTI d'une source spécifique
func GetCTIDataBySource(source string) ([]CTIData, error) {
    filter := bson.M{"source": source}
    cursor, err := CTICollection.Find(context.Background(), filter)
    if err != nil {
        return nil, err
    }
    defer cursor.Close(context.Background())

    var results []CTIData
    if err := cursor.All(context.Background(), &results); err != nil {
        return nil, err
    }

    return results, nil
}
