package db

import (
	"context"
	"time"
)

// RecordSeedDiscovery shares the frontier's canonical URL identity.
func RecordSeedDiscovery(ctx context.Context, raw, source string) error {
	key := "crawler:page:" + cleanOnionURL(raw)
	pipe := RedisClient.TxPipeline()
	pipe.HSet(ctx, key, "seed_source", source, "seed_discovered_at", time.Now().Unix())
	pipe.Expire(ctx, key, 90*24*time.Hour)
	_, err := pipe.Exec(ctx)
	return err
}
