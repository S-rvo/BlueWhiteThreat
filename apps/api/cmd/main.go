package main

import (
	"context"
	"errors"
	"github.com/S-rvo/BlueWhiteThreat/pkg/contract"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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
		db = contract.Database
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

	server := &http.Server{
		Addr:              ":8083",
		Handler:           newHandler(mongoStore{client.Database(db).Collection(contract.ArticleCollection)}),
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
