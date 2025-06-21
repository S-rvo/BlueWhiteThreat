package db

import (
	"context"
	"log"
	"regexp"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

var driver neo4j.DriverWithContext

// Regex pour extraire uniquement l'URL principale
var onionDomainRegex = regexp.MustCompile(`^(https?://[^/]+)`)

// Nettoyer l'URL pour garder seulement "http(s)://xxx.onion"
func cleanOnionURL(url string) string {
	matches := onionDomainRegex.FindStringSubmatch(url)
	if len(matches) > 1 {
		return matches[1]
	}
	return url
}

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
		if err := driver.Close(context.Background()); err != nil {
			log.Printf("Error closing Neo4j driver: %v", err)
		}
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

// FetchGraphRelations récupère toutes les relations nettoyées (URL principale uniquement)
func FetchGraphRelations() ([]map[string]interface{}, error) {
	session := driver.NewSession(context.Background(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(context.Background())

	result, err := session.ExecuteRead(context.Background(), func(tx neo4j.ManagedTransaction) (any, error) {
		recs, err := tx.Run(context.Background(),
			"MATCH (a:URL)-[:LINKS_TO]->(b:URL) RETURN a.url AS source, b.url AS target",
			nil)
		if err != nil {
			return nil, err
		}

		// Utiliser une map pour regrouper par source
		grouped := make(map[string][]string)

		for recs.Next(context.Background()) {
			rec := recs.Record()
			src, _ := rec.Get("source")
			tgt, _ := rec.Get("target")

			sourceURL := cleanOnionURL(src.(string))
			targetURL := cleanOnionURL(tgt.(string))

			grouped[sourceURL] = append(grouped[sourceURL], targetURL)
		}

		var links []map[string]interface{}
		for src, targets := range grouped {
			links = append(links, map[string]interface{}{
				"source":  src,
				"targets": targets,
			})
		}

		return links, nil
	})

	if err != nil {
		return nil, err
	}
	return result.([]map[string]interface{}), nil
}

// FetchAllURLs retourne toutes les URLs principales (nettoyées) sans erreurs de type
func FetchAllURLs() ([]string, error) {
	session := driver.NewSession(context.Background(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(context.Background())

	var urls []string

	_, err := session.ExecuteRead(context.Background(), func(tx neo4j.ManagedTransaction) (any, error) {
		recs, err := tx.Run(context.Background(),
			"MATCH (u:URL) RETURN u.url AS url",
			nil)
		if err != nil {
			return nil, err
		}

		for recs.Next(context.Background()) {
			rec := recs.Record()
			urlValue, found := rec.Get("url")
			if !found {
				continue
			}
			if urlStr, ok := urlValue.(string); ok {
				cleaned := cleanOnionURL(urlStr)
				urls = append(urls, cleaned)
			}
		}
		return nil, nil
	})

	if err != nil {
		return nil, err
	}
	return urls, nil
}
