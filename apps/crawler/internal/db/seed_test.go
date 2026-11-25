package db

import "testing"

func TestSeedProvenanceUsesFrontierIdentity(t *testing.T) {
	setupTestRedisClient(t)
	if err := RecordSeedDiscovery(ctx, "http://EXAMPLE.onion/", "source"); err != nil {
		t.Fatal(err)
	}
	if RedisClient.HGet(ctx, "crawler:page:http://example.onion", "seed_source").Val() != "source" {
		t.Fatal("wrong canonical metadata key")
	}
}
