package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.SetTrustedProxies([]string{"127.0.0.1"})
	r.Use(cors.Default())

	r.GET("/data", func(c *gin.Context) {
		client, err := mongo.NewClient(options.Client().ApplyURI("mongodb://root:root@mongodb:27017"))
		if err != nil {
			log.Printf("Erreur création client MongoDB: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Erreur connexion MongoDB"})
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := client.Connect(ctx); err != nil {
			log.Printf("Erreur connexion MongoDB: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Erreur connexion MongoDB"})
			return
		}
		defer client.Disconnect(ctx)

		collection := client.Database("BlueWhiteThreat").Collection("article")
		cur, err := collection.Find(ctx, map[string]interface{}{})
		if err != nil {
			log.Printf("Erreur Find: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Erreur lecture données", "details": err.Error()})
			return
		}
		defer cur.Close(ctx)

		var results []map[string]interface{}
		for cur.Next(ctx) {
			var result map[string]interface{}
			if err := cur.Decode(&result); err != nil {
				log.Printf("Erreur decode: %v", err)
				continue
			}
			results = append(results, result)
		}
		c.JSON(http.StatusOK, results)
	})

	r.Run(":8083")
} 