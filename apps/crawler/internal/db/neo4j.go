package db

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

var driver neo4j.DriverWithContext

// InitNeo4jDriver initializes the Neo4j driver with the given URI, username, and password.
func InitNeo4jDriver(uri, username, password string) error {
	var err error
	driver, err = neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(username, password, ""))
	if err != nil {
		return err
	}
	if err := EnsureGraphSchema(); err != nil {
		driver.Close(context.Background())
		return err
	}
	return nil
}

// SaveURL crée un nœud URL s'il n'existe pas déjà
func SaveURL(ctx context.Context, url string) error {
	c, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cleanedURL := cleanOnionURL(url)
	session := driver.NewSession(c, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(c)

	_, err := session.ExecuteWrite(c, func(tx neo4j.ManagedTransaction) (any, error) {
		result, err := tx.Run(c,
			"MERGE (u:URL {url: $url})",
			map[string]interface{}{"url": cleanedURL})
		if err != nil {
			return nil, err
		}
		_, err = result.Consume(c)
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

func cleanOnionURL(raw string) string {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Fragment = ""
	if parsed.Path == "/" {
		parsed.Path = ""
	}
	return parsed.String()
}

// SaveLinks persists a page's edges in bounded transactions.
func SaveLinks(ctx context.Context, source string, links []string) error {
	for offset := 0; offset < len(links); offset += 250 {
		end := offset + 250
		if end > len(links) {
			end = len(links)
		}
		batch := make([]string, 0, end-offset)
		for _, link := range links[offset:end] {
			batch = append(batch, cleanOnionURL(link))
		}
		if err := saveLinkBatch(ctx, source, batch); err != nil {
			return err
		}
	}
	return nil
}
func saveLinkBatch(ctx context.Context, source string, links []string) error {
	c, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	session := driver.NewSession(c, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(c)
	_, err := session.ExecuteWrite(c, func(tx neo4j.ManagedTransaction) (any, error) {
		result, err := tx.Run(c, "MERGE (src:URL {url: $source}) WITH src UNWIND $links AS link MERGE (tgt:URL {url: link}) MERGE (src)-[:LINKS_TO]->(tgt)", map[string]interface{}{"source": cleanOnionURL(source), "links": links})
		if err != nil {
			return nil, err
		}
		_, err = result.Consume(c)
		return nil, err
	})
	return err
}

func Ready() error {
	c, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if RedisClient == nil || driver == nil {
		return fmt.Errorf("databases not initialized")
	}
	if err := RedisClient.Ping(c).Err(); err != nil {
		return err
	}
	return driver.VerifyConnectivity(c)
}

// EnsureGraphSchema is migration v1. Existing duplicates cause an actionable
// startup error; no existing nodes are removed or merged automatically.
func EnsureGraphSchema() error {
	c, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	session := driver.NewSession(c, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(c)
	result, err := session.Run(c, "CREATE CONSTRAINT crawler_url_unique IF NOT EXISTS FOR (u:URL) REQUIRE u.url IS UNIQUE", nil)
	if err == nil {
		_, err = result.Consume(c)
	}
	if err != nil {
		return fmt.Errorf("crawler schema v1 (URL.url uniqueness): inspect duplicate URLs before migration: %w", err)
	}
	return nil
}
