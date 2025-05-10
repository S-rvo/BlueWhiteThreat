package db

import (
	"context"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

var driver neo4j.DriverWithContext

func InitNeo4j(uri, user, password string) error {
	var err error
	driver, err = neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(user, password, ""))
	return err
}

func CloseNeo4j() {
	driver.Close(context.Background())
}

func FetchGraphRelations() ([]map[string]string, error) {
	session := driver.NewSession(context.Background(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(context.Background())

	result, err := session.ExecuteRead(context.Background(), func(tx neo4j.ManagedTransaction) (any, error) {
		recs, err := tx.Run(context.Background(), "MATCH (a:URL)-[:LINKS_TO]->(b:URL) RETURN a.url AS source, b.url AS target", nil)
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
