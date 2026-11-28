package db

import (
	"context"
	"fmt"
	"sort"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"go.mongodb.org/mongo-driver/bson/primitive"

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

// URLIndicatorLabel porte les URL extraites comme IOC. Le label URL appartient
// au crawler (clé url, contrainte crawler_url_unique) : le réutiliser mélangeait
// deux modèles de noeud sous un même label.
const URLIndicatorLabel = "URLIndicator"

// entityLabels est la liste fermée des labels d'entité. Les labels sont injectés
// dans le Cypher : ils ne doivent jamais provenir d'une donnée externe.
var entityLabels = []string{"IP", "Hash", "Domain", URLIndicatorLabel, "CVE", "Malware", "ThreatActor", "Infrastructure"}

// entityGroups associe un label Neo4j à la liste de valeurs correspondante.
func entityGroups(e entities.Entities) map[string][]string {
	return map[string][]string{
		"IP":              e.IOCs.IPs,
		"Hash":            e.IOCs.Hashes,
		"Domain":          e.IOCs.Domains,
		URLIndicatorLabel: e.IOCs.URLs,
		"CVE":             e.CVEs,
		"Malware":         e.Malware,
		"ThreatActor":     e.ThreatActors,
		"Infrastructure":  e.Infrastructure,
	}
}

// ArticleKey est l'identifiant stable d'un article dans le graphe : l'hexadécimal
// d'un ObjectID, sinon la valeur telle quelle. fmt.Sprint produisait
// `ObjectID("…")`, un format dépendant du driver.
func ArticleKey(id any) string {
	switch v := id.(type) {
	case primitive.ObjectID:
		return v.Hex()
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

// EnsureGraphSchema migre les données des versions précédentes puis pose les
// contraintes d'unicité. Idempotent : à appeler au démarrage.
func EnsureGraphSchema(ctx context.Context) error {
	session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	migrations := []string{
		// Les IOC URL étaient écrits sous le label du crawler. Ils se distinguent
		// des noeuds de crawl par la clé value (le crawler n'écrit que url).
		`MATCH (n:URL) WHERE n.value IS NOT NULL AND n.url IS NULL
		 SET n:` + URLIndicatorLabel + ` REMOVE n:URL`,
		// `ObjectID("<24 hex>")` -> `<24 hex>`.
		`MATCH (a:Article) WHERE a.id STARTS WITH 'ObjectID("' AND a.id ENDS WITH '")'
		 SET a.id = substring(a.id, 10, size(a.id) - 12)`,
	}
	for _, query := range migrations {
		if _, err := session.Run(ctx, query, nil); err != nil {
			return fmt.Errorf("neo4j: migration: %w", err)
		}
	}

	constraints := []string{
		`CREATE CONSTRAINT parser_article_id IF NOT EXISTS FOR (a:Article) REQUIRE a.id IS UNIQUE`,
	}
	for _, label := range entityLabels {
		constraints = append(constraints, fmt.Sprintf(
			`CREATE CONSTRAINT parser_%s_value IF NOT EXISTS FOR (e:%s) REQUIRE e.value IS UNIQUE`,
			label, label))
	}
	for _, query := range constraints {
		if _, err := session.Run(ctx, query, nil); err != nil {
			return fmt.Errorf("neo4j: constraint: %w", err)
		}
	}
	return nil
}

// ReplaceEntityGraph remplace, en une seule transaction, les entités rattachées
// à un article : les relations MENTIONED_IN existantes sont supprimées (et les
// entités devenues orphelines avec elles), puis les entités courantes sont
// écrites. Des entités vides retirent l'article du graphe. Rejouer la même
// écriture est sans effet.
func ReplaceEntityGraph(ctx context.Context, articleID string, e entities.Entities) error {
	session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	groups := entityGroups(e)
	labels := make([]string, 0, len(groups))
	for label, values := range groups {
		if len(values) > 0 {
			labels = append(labels, label)
		}
	}
	sort.Strings(labels)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		params := map[string]any{"articleID": articleID}
		if _, err := tx.Run(ctx, `
			MATCH (a:Article {id: $articleID})<-[r:MENTIONED_IN]-(e)
			DELETE r
			WITH DISTINCT e
			WHERE NOT (e)--()
			DELETE e`, params); err != nil {
			return nil, err
		}
		for _, label := range labels {
			query := `MERGE (a:Article {id: $articleID})
				WITH a
				UNWIND $values AS value
				MERGE (e:` + label + ` {value: value})
				MERGE (e)-[:MENTIONED_IN]->(a)`
			if _, err := tx.Run(ctx, query, map[string]any{"articleID": articleID, "values": groups[label]}); err != nil {
				return nil, err
			}
		}
		if len(labels) == 0 {
			if _, err := tx.Run(ctx, `MATCH (a:Article {id: $articleID}) WHERE NOT (a)--() DELETE a`, params); err != nil {
				return nil, err
			}
		}
		return nil, nil
	})
	return err
}

// RetractEntityGraph retire un article et ses entités orphelines du graphe.
func RetractEntityGraph(ctx context.Context, articleID string) error {
	return ReplaceEntityGraph(ctx, articleID, entities.Entities{})
}
