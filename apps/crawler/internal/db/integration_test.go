package db

import (
	"fmt"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/redis/go-redis/v9"
	"os"
	"sync"
	"testing"
	"time"
)

// Only the disposable-service script sets these variables. Never use production
// REDIS_HOST or NEO4J_URI implicitly for an integration test.
func TestDisposableIntegration(t *testing.T) {
	addr := os.Getenv("CRAWLER_TEST_REDIS")
	uri := os.Getenv("CRAWLER_TEST_NEO4J")
	if addr == "" || uri == "" {
		t.Skip("run scripts/integration.sh")
	}
	RedisClient = redis.NewClient(&redis.Options{Addr: addr})
	defer func() { RedisClient.Close(); RedisClient = nil }()
	if err := InitNeo4jDriver(uri, "neo4j", "fixture-password"); err != nil {
		t.Fatal(err)
	}
	defer CloseNeo4j()
	if err := Ready(); err != nil {
		t.Fatal(err)
	}
	if err := SetupRedisQueues(); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	for i := 0; i < 100; i++ {
		u := fmt.Sprintf("http://fixture%d.onion/page", i)
		if _, err := AddURLToQueue(u); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 100; i++ {
		u, token, err := ClaimURL()
		if err != nil || u == "" {
			t.Fatalf("claim: %s %v", u, err)
		}
		if err := SaveLinks(u, []string{"http://destination.onion/page"}); err != nil {
			t.Fatal(err)
		}
		if err := SaveClaimMetadata(u, token, "", "", "same-content"); err != nil {
			t.Fatal(err)
		}
		if err := FinishClaim(u, token, true, time.Hour, 200); err != nil {
			t.Fatal(err)
		}
	}
	count, err := GetCrawledCount()
	if err != nil || count != 100 {
		t.Fatalf("count %d %v", count, err)
	}
	t.Logf("100 durable tasks + Neo4j edges: %s", time.Since(started))
	for i := 0; i < 100; i++ {
		if _, err := AddURLToQueue(fmt.Sprintf("http://parallel%d.onion/page", i)); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	errors := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				u, token, err := ClaimURL()
				if err != nil {
					errors <- err
					return
				}
				if u == "" {
					return
				}
				if err := SaveLinks(u, []string{"http://destination.onion/page"}); err != nil {
					errors <- err
					return
				}
				if err := FinishClaim(u, token, true, time.Hour, 200); err != nil {
					errors <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	count, err = GetCrawledCount()
	if err != nil || count != 200 {
		t.Fatalf("parallel count %d %v", count, err)
	}
	t.Log("4 concurrent workers: 100 additional durable tasks completed")

	if err := EnsureGraphSchema(); err != nil {
		t.Fatal(err)
	}
	var writers sync.WaitGroup
	for i := 0; i < 12; i++ {
		writers.Add(1)
		go func() {
			defer writers.Done()
			if err := SaveURL("http://unique-fixture.onion"); err != nil {
				t.Error(err)
			}
		}()
	}
	writers.Wait()
	session := driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)
	result, err := session.Run(ctx, "MATCH (u:URL {url: $url}) RETURN count(u) AS n", map[string]interface{}{"url": "http://unique-fixture.onion"})
	if err != nil {
		t.Fatal(err)
	}
	record, err := result.Single(ctx)
	if err != nil {
		t.Fatal(err)
	}
	n, _ := record.Get("n")
	if n != int64(1) {
		t.Fatalf("duplicate graph nodes: %v", n)
	}
	t.Log("URL uniqueness schema idempotent; 12 concurrent MERGE writers produce one node")

}
