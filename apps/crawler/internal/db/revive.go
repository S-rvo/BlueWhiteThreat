package db

import (
	"context"
	"fmt"
	"time"
)

// ReviveDeadURLs schedules a bounded sample of old failures atomically. It
// never marks content fresh or bypasses the worker's filters/host budgets.
func ReviveDeadURLs(ctx context.Context, minAge time.Duration, limit int) (int, error) {
	if limit < 1 || limit > 100 || minAge < 7*24*time.Hour {
		return 0, fmt.Errorf("revival requires age >=7 days and limit 1..100")
	}
	now := time.Now()
	return RedisClient.Eval(ctx, `
 local urls=redis.call('ZRANGEBYSCORE',KEYS[1],'-inf',ARGV[1],'LIMIT',0,ARGV[2])
 local n=0
 for _,u in ipairs(urls) do
  if not redis.call('ZSCORE','crawler:inflight',u) and redis.call('SISMEMBER','urls_to_crawl',u)==0 and not redis.call('ZSCORE',KEYS[2],u) then
   redis.call('ZREM',KEYS[1],u)
   redis.call('HDEL','crawler:attempts',u)
   redis.call('ZADD',KEYS[2],ARGV[3],u)
   n=n+1
  end
 end
 redis.call('HINCRBY','crawler:stats','dead_revived_total',n)
 return n`, []string{"crawler:dead", "crawler:scheduled"}, now.Add(-minAge).Unix(), limit, now.Unix()).Int()
}
