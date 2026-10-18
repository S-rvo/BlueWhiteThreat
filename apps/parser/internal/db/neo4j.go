package db

import (
	"context"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/entities"
)

var driver neo4j.DriverWithContext

func InitNeo4jDriver(uri, username, password string) error {
	var err error
	driver, err = neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(username, password, ""))
	return err
}

func CloseNeo4j() {
	if driver != nil {
		driver.Close(context.Background())
	}
}

// entityGroups associe un label Neo4j à la liste de valeurs correspondante.
func entityGroups(e entities.Entities) map[string][]string {
	return map[string][]string{
		"IP":             e.IOCs.IPs,
		"Hash":           e.IOCs.Hashes,
		"Domain":         e.IOCs.Domains,
		"URL":            e.IOCs.URLs,
		"CVE":            e.CVEs,
		"Malware":        e.Malware,
		"ThreatActor":    e.ThreatActors,
		"Infrastructure": e.Infrastructure,
	}
}

// SaveEntityGraph crée un noeud Article (si absent), un noeud par entité typée,
// et une relation MENTIONED_IN entre chaque entité et l'article source.
func SaveEntityGraph(ctx context.Context, articleID string, e entities.Entities) error {
	session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	for label, values := range entityGroups(e) {
		for _, v := range values {
			_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
				query := `MERGE (a:Article {id: $articleID})
                          MERGE (e:` + label + ` {value: $value})
                          MERGE (e)-[:MENTIONED_IN]->(a)`
				_, err := tx.Run(ctx, query, map[string]any{"articleID": articleID, "value": v})
				return nil, err
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}
