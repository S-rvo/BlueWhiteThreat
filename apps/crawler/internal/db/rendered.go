package db

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/redis/go-redis/v9"
	"time"
)

// Claims survive worker crashes. Failed events move to the tail, then a bounded
// dead-letter list; one poison event must not block discovery for other pages.
func DrainRenderedLinks(ctx context.Context) error { return drainRenderedLinks(ctx, SaveLinks) }
func drainRenderedLinks(ctx context.Context, save func(context.Context, string, []string) error) error {
	var failures []error
	for i := 0; i < 100; i++ {
		tokenBytes := make([]byte, 16)
		if _, err := rand.Read(tokenBytes); err != nil {
			return err
		}
		token := hex.EncodeToString(tokenBytes)
		raw, err := RedisClient.Eval(ctx, `
local due=redis.call('ZRANGEBYSCORE','crawler:rendered_retry','-inf',ARGV[2],'LIMIT',0,100)
for _,v in ipairs(due) do redis.call('ZREM','crawler:rendered_retry',v);redis.call('RPUSH',KEYS[1],v) end
local expired=redis.call('ZRANGEBYSCORE','crawler:rendered_leases','-inf',ARGV[2],'LIMIT',0,100)
for _,t in ipairs(expired) do
 local v=redis.call('HGET','crawler:rendered_inflight',t)
 if v then redis.call('RPUSH',KEYS[1],v) end
 redis.call('HDEL','crawler:rendered_inflight',t); redis.call('ZREM','crawler:rendered_leases',t)
end
local v=redis.call('LPOP',KEYS[1]); if not v then return false end
redis.call('HSET','crawler:rendered_inflight',ARGV[1],v)
redis.call('ZADD','crawler:rendered_leases',tonumber(ARGV[2])+600,ARGV[1]);return v
`, []string{"crawler:rendered_links"}, token, time.Now().Unix()).Text()
		if err == redis.Nil {
			break
		}
		if err != nil {
			return err
		}
		var event struct {
			Source string
			Links  []string
		}
		if len(raw) > 64*1024 {
			err = fmt.Errorf("oversized rendered event")
		} else {
			err = json.Unmarshal([]byte(raw), &event)
		}
		if err == nil {
			reason, e := AddURLToQueue(ctx, event.Source)
			err = e
			if err == nil && (reason == "storage" || reason == "trap" || reason == QueueReasonBlocked || reason == QueueReasonFarm) {
				err = fmt.Errorf("rendered source not admitted: %s", reason)
			}
		}
		if err == nil {
			if _, valid := discoveryKey(event.Source); !valid {
				err = fmt.Errorf("invalid rendered source")
			} else {
				accepted := make([]string, 0, len(event.Links))
				for _, link := range event.Links {
					reason, e := AddURLToQueue(ctx, link)
					if e != nil {
						err = e
						break
					}
					if reason != "storage" && reason != "trap" && reason != QueueReasonBlocked && reason != QueueReasonFarm {
						accepted = append(accepted, link)
					}
				}
				if err == nil && len(accepted) > 0 {
					err = save(ctx, event.Source, accepted)
				}
			}
		}
		failed := 0
		if err != nil {
			failed = 1
			failures = append(failures, err)
		}
		// SHA1 here is an identity/deduplication key, not a security primitive.
		e := RedisClient.Eval(ctx, `
local v=redis.call('HGET','crawler:rendered_inflight',ARGV[1]);if not v then return 0 end
redis.call('HDEL','crawler:rendered_inflight',ARGV[1]);redis.call('ZREM','crawler:rendered_leases',ARGV[1])
local id=redis.sha1hex(v)
if ARGV[2]=='1' then
 local n=redis.call('HINCRBY','crawler:rendered_attempts',id,1)
 if n>=5 then
  redis.call('LPUSH','crawler:rendered_dead',v);redis.call('LTRIM','crawler:rendered_dead',0,511)
  redis.call('HDEL','crawler:rendered_attempts',id)
 else redis.call('ZADD','crawler:rendered_retry',tonumber(ARGV[3])+math.min(300,2^n),v) end
else redis.call('HDEL','crawler:rendered_attempts',id) end
return 1`, []string{"crawler:rendered_links"}, token, failed, time.Now().Unix()).Err()
		if e != nil {
			return e
		}
	}
	return errors.Join(failures...)
}
