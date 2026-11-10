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
	return err
}

// SaveURL crée un nœud URL s'il n'existe pas déjà
func SaveURL(url string) error {
	cleanedURL := cleanOnionURL(url)
	session := driver.NewSession(context.Background(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(context.Background())

	_, err := session.ExecuteWrite(context.Background(), func(tx neo4j.ManagedTransaction) (any, error) {
		result, err := tx.Run(context.Background(),
			"MERGE (u:URL {url: $url})",
			map[string]interface{}{"url": cleanedURL})
		if err != nil {
			return nil, err
		}
		_, err = result.Consume(context.Background())
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
		result, err := tx.Run(context.Background(),
			`MATCH (src:URL {url: $source})
			 MERGE (tgt:URL {url: $target})
			 MERGE (src)-[:LINKS_TO]->(tgt)`,
			map[string]interface{}{"source": cleanedSource, "target": cleanedTarget})
		if err != nil {
			return nil, err
		}
		_, err = result.Consume(context.Background())
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
func SaveLinks(source string, links []string) error {
	for offset := 0; offset < len(links); offset += 250 {
		end := offset + 250
		if end > len(links) {
			end = len(links)
		}
		batch := make([]string, 0, end-offset)
		for _, link := range links[offset:end] {
			batch = append(batch, cleanOnionURL(link))
		}
		if err := saveLinkBatch(source, batch); err != nil {
			return err
		}
	}
	return nil
}
func saveLinkBatch(source string, links []string) error {
	c, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
