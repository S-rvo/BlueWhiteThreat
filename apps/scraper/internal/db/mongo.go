package db

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
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

// Pour enregistrer chaque TableEntry individuellement en upsert
func SaveAllEntries(base, collection string, entries interface{}, uniqueFields []string) (int, error) {
    client, err := ConnectMongo()
    if err != nil {
        return 0, err
    }
    defer client.Disconnect(context.TODO())
    coll := client.Database(base).Collection(collection)

    // On convertit entries en slice de valeurs
    slice := []interface{}{}
    val := reflect.ValueOf(entries)
    if val.Kind() == reflect.Slice {
        for i := 0; i < val.Len(); i++ {
            slice = append(slice, val.Index(i).Interface())
        }
    } else {
        return 0, fmt.Errorf("entries doit être un slice")
    }

    inserted := 0
    for _, entry := range slice {
        filter := bson.M{}
        // On construit le filtre unique dynamiquement
        for _, field := range uniqueFields {
            // On utilise la réflexion pour lire la valeur du champ
            val := reflect.ValueOf(entry)
            if val.Kind() == reflect.Struct {
                f := val.FieldByName(field)
                if f.IsValid() {
                    filter[strings.ToLower(field)] = f.Interface()
                }
            }
        }
        update := bson.M{"$set": entry}
        res, err := coll.UpdateOne(context.TODO(), filter, update, options.Update().SetUpsert(true))
        if err != nil {
            return inserted, err
        }
        if res.MatchedCount == 0 && res.UpsertedCount == 1 {
            inserted++
        }
    }
    return inserted, nil
}
