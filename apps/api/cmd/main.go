package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j/config"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	uri := os.Getenv("MONGODB_URI")
	if uri == "" {
		log.Fatal("MONGODB_URI is required; use a dedicated read-only MongoDB account")
	}
	db := os.Getenv("MONGO_DB")
	if db == "" {
		db = "BlueWhiteThreat"
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri).
		SetConnectTimeout(3*time.Second).SetServerSelectionTimeout(3*time.Second))
	if err != nil {
		log.Fatal("invalid MongoDB configuration")
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := client.Disconnect(closeCtx); err != nil {
			log.Print("could not close MongoDB client cleanly")
		}
	}()

	var graph graphStore = unavailableGraph{}
	if neoURI := os.Getenv("NEO4J_URI"); neoURI != "" {
		user := os.Getenv("NEO4J_USER")
		if user == "" {
			user = "neo4j"
		}
		driver, err := neo4j.NewDriverWithContext(neoURI, neo4j.BasicAuth(user, os.Getenv("NEO4J_PASSWORD"), ""),
			func(c *config.Config) {
				c.SocketConnectTimeout = 3 * time.Second
				c.ConnectionAcquisitionTimeout = 3 * time.Second
			})
		if err != nil {
			log.Fatal("invalid Neo4j configuration")
		}
		defer func() {
			closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = driver.Close(closeCtx)
		}()
		graph = neo4jGraph{driver}
	} else {
		log.Print("NEO4J_URI not set: /graph will answer 503")
	}

	server := &http.Server{
		Addr:              ":8083",
		Handler:           newHandler(mongoStore{client.Database(db).Collection("article")}, graph),
		ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      8 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	select {
	case err := <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	case <-ctx.Done():
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(closeCtx); err != nil {
			log.Printf("API shutdown: %v", err)
		}
	}
}
