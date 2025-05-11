package db

import (
	"context"
	"log"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

var driver neo4j.DriverWithContext

// InitNeo4j initialise la connexion à Neo4j
func InitNeo4j(uri, user, password string) error {
	var err error
	driver, err = neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(user, password, ""))
	if err == nil {
		log.Println("Connected to Neo4j")
	}
	return err
}

// CloseNeo4j ferme proprement la connexion Neo4j
func CloseNeo4j() {
	if driver != nil {
		driver.Close(context.Background())
	}
}

// SaveURL crée un nœud URL s'il n'existe pas déjà
func SaveURL(url string) error {
	session := driver.NewSession(context.Background(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(context.Background())

	_, err := session.ExecuteWrite(context.Background(), func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(context.Background(),
			"MERGE (u:URL {url: $url})",
			map[string]interface{}{"url": url})
		return nil, err
	})
	return err
}

// SaveLink crée une relation LINKS_TO entre deux URLs
func SaveLink(sourceURL, targetURL string) error {
	session := driver.NewSession(context.Background(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(context.Background())

	_, err := session.ExecuteWrite(context.Background(), func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(context.Background(),
			`MATCH (src:URL {url: $source})
			 MERGE (tgt:URL {url: $target})
			 MERGE (src)-[:LINKS_TO]->(tgt)`,
			map[string]interface{}{"source": sourceURL, "target": targetURL})
		return nil, err
	})
	return err
}

// FetchGraphRelations récupère toutes les relations URL ➔ URL
func FetchGraphRelations() ([]map[string]string, error) {
	session := driver.NewSession(context.Background(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(context.Background())

	result, err := session.ExecuteRead(context.Background(), func(tx neo4j.ManagedTransaction) (any, error) {
		recs, err := tx.Run(context.Background(),
			"MATCH (a:URL)-[:LINKS_TO]->(b:URL) RETURN a.url AS source, b.url AS target",
			nil)
		if err != nil {
			return nil, err
		}

		var links []map[string]string
		for recs.Next(context.Background()) {
			rec := recs.Record()
			src, _ := rec.Get("source")
			tgt, _ := rec.Get("target")
			links = append(links, map[string]string{"source": src.(string), "target": tgt.(string)})
		}
		return links, nil
	})

	if err != nil {
		return nil, err
	}
	return result.([]map[string]string), nil
}

// FetchAllURLs retourne toutes les URLs enregistrées dans Neo4j
func FetchAllURLs() ([]string, error) {
	session := driver.NewSession(context.Background(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(context.Background())

	result, err := session.ExecuteRead(context.Background(), func(tx neo4j.ManagedTransaction) (any, error) {
		recs, err := tx.Run(context.Background(),
			"MATCH (u:URL) RETURN u.url AS url",
			nil)
		if err != nil {
			return nil, err
		}

		var urls []string
		for recs.Next(context.Background()) {
			rec := recs.Record()
			url, _ := rec.Get("url")
			urls = append(urls, url.(string))
		}
		return urls, nil
	})

	if err != nil {
		return nil, err
	}
	return result.([]string), nil
}
