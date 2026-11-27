package main

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// Graphe CTI projeté par le parser : (entité)-[:MENTIONED_IN]->(:Article {id}).
// Le graphe d'URL du crawler n'est volontairement pas exposé : ce sont des URL
// sources, exclues de toute réponse de l'API. Pour la même raison, les IOC de
// type URL (label URLIndicator) sont écartés.

// graphEntityKinds est la liste fermée des labels exposés.
var graphEntityKinds = map[string]bool{
	"IP": true, "Hash": true, "Domain": true, "CVE": true,
	"Malware": true, "ThreatActor": true, "Infrastructure": true,
}

const (
	defaultGraphArticles = 100
	maxGraphArticles     = 200
	// Plafond d'entités par article : borne la réponse à 200 × 10 arêtes.
	maxEntitiesPerArticle = 10
)

type graphEntity struct {
	Kind  string
	Value string
}

type graphArticle struct {
	ID       string
	Entities []graphEntity
}

type graphStore interface {
	articles(ctx context.Context, limit int) ([]graphArticle, error)
}

type graphNode struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Label string `json:"label"`
}

type graphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type graphResponse struct {
	Nodes []graphNode `json:"nodes"`
	Edges []graphEdge `json:"edges"`
}

// buildGraph dédoublonne les noeuds (une entité citée par plusieurs articles
// n'apparaît qu'une fois) et réapplique les bornes et la liste fermée de labels,
// sans faire confiance au store.
func buildGraph(rows []graphArticle, limit int) graphResponse {
	out := graphResponse{Nodes: []graphNode{}, Edges: []graphEdge{}}
	seen := map[string]bool{}
	if len(rows) > limit {
		rows = rows[:limit]
	}
	for _, row := range rows {
		if row.ID == "" {
			continue
		}
		article := "Article:" + row.ID
		if !seen[article] {
			seen[article] = true
			out.Nodes = append(out.Nodes, graphNode{ID: article, Kind: "Article", Label: row.ID})
		}
		entities := row.Entities
		if len(entities) > maxEntitiesPerArticle {
			entities = entities[:maxEntitiesPerArticle]
		}
		for _, e := range entities {
			if !graphEntityKinds[e.Kind] || e.Value == "" {
				continue
			}
			id := e.Kind + ":" + e.Value
			if !seen[id] {
				seen[id] = true
				out.Nodes = append(out.Nodes, graphNode{ID: id, Kind: e.Kind, Label: e.Value})
			}
			out.Edges = append(out.Edges, graphEdge{From: id, To: article})
		}
	}
	return out
}

func graphLimit(raw string) (int, bool) {
	q, err := url.ParseQuery(raw)
	if err != nil {
		return 0, false
	}
	values, exists := q["limit"]
	if !exists {
		return defaultGraphArticles, true
	}
	if len(values) != 1 {
		return 0, false
	}
	limit, err := strconv.Atoi(values[0])
	if err != nil || limit < 1 || limit > maxGraphArticles {
		return 0, false
	}
	return limit, true
}

var errGraphUnavailable = errors.New("graph not configured")

// unavailableGraph est utilisé quand NEO4J_URI n'est pas défini : /graph
// répond 503 sans empêcher /data de fonctionner.
type unavailableGraph struct{}

func (unavailableGraph) articles(context.Context, int) ([]graphArticle, error) {
	return nil, errGraphUnavailable
}

type neo4jGraph struct{ driver neo4j.DriverWithContext }

// Les articles les plus récents d'abord : l'id est l'hexadécimal d'un ObjectID,
// dont l'ordre lexicographique suit la date de création.
const graphQuery = `
MATCH (a:Article)
WITH a ORDER BY a.id DESC LIMIT $limit
CALL {
	WITH a
	MATCH (e)-[:MENTIONED_IN]->(a)
	WHERE any(label IN labels(e) WHERE label IN $kinds)
	WITH e ORDER BY e.value LIMIT $perArticle
	RETURN collect({kind: [label IN labels(e) WHERE label IN $kinds][0], value: e.value}) AS entities
}
RETURN a.id AS id, entities`

func (g neo4jGraph) articles(ctx context.Context, limit int) ([]graphArticle, error) {
	kinds := make([]string, 0, len(graphEntityKinds))
	for kind := range graphEntityKinds {
		kinds = append(kinds, kind)
	}
	session := g.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, graphQuery, map[string]any{
			"limit": limit, "perArticle": maxEntitiesPerArticle, "kinds": kinds,
		})
		if err != nil {
			return nil, err
		}
		records, err := res.Collect(ctx)
		if err != nil {
			return nil, err
		}
		rows := make([]graphArticle, 0, len(records))
		for _, record := range records {
			rawID, _ := record.Get("id")
			raw, _ := record.Get("entities")
			id, _ := rawID.(string)
			row := graphArticle{ID: id}
			list, _ := raw.([]any)
			for _, item := range list {
				m, _ := item.(map[string]any)
				kind, _ := m["kind"].(string)
				value, _ := m["value"].(string)
				row.Entities = append(row.Entities, graphEntity{Kind: kind, Value: value})
			}
			rows = append(rows, row)
		}
		return rows, nil
	}, neo4j.WithTxTimeout(3*time.Second))
	if err != nil {
		return nil, err
	}
	return result.([]graphArticle), nil
}
