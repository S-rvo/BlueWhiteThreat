package db

import (
	"context"
	"log"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

var driver neo4j.DriverWithContext

// InitNeo4jDriver initializes the Neo4j driver with the given URI, username, and password.
func InitNeo4jDriver(uri, username, password string) error {
	var err error
	driver, err = neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(username, password, ""))
	return err
}

// SaveURL crée un nœud URL s'il n'existe pas déjà
func SaveURL(url string) error {
	cleanedURL := cleanOnionURL(url)
	session := driver.NewSession(context.Background(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(context.Background())

	_, err := session.ExecuteWrite(context.Background(), func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(context.Background(),
			"MERGE (u:URL {url: $url})",
			map[string]interface{}{"url": cleanedURL})
		return nil, err
	})
	return err
}

// SaveLink crée une relation LINKS_TO entre deux URLs
func SaveLink(sourceURL, targetURL string) error {
	cleanedSource := cleanOnionURL(sourceURL)
	cleanedTarget := cleanOnionURL(targetURL)
	session := driver.NewSession(context.Background(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(context.Background())

	_, err := session.ExecuteWrite(context.Background(), func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(context.Background(),
			`MATCH (src:URL {url: $source})
			 MERGE (tgt:URL {url: $target})
			 MERGE (src)-[:LINKS_TO]->(tgt)`,
			map[string]interface{}{"source": cleanedSource, "target": cleanedTarget})
		return nil, err
	})
	return err
}

// CloseNeo4j ferme proprement la connexion Neo4j
func CloseNeo4j() {
	if driver != nil {
		if err := driver.Close(context.Background()); err != nil {
			log.Printf("Error closing Neo4j driver: %v", err)
		}
	}
}
