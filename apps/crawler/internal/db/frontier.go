package db

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// ClaimURL atomically moves a ready task into a ten minute lease. Expired
// leases are recovered before selection; completion requires the lease token.
func ClaimURL() (string, string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", "", err
	}
	token := hex.EncodeToString(random[:])
	v, err := RedisClient.Eval(ctx, `
 local now=tonumber(ARGV[1])
 for _,u in ipairs(redis.call('ZRANGEBYSCORE',KEYS[3],'-inf',now,'LIMIT',0,200)) do
  redis.call('ZREM',KEYS[3],u); redis.call('HDEL',KEYS[4],u)
  if redis.call('SCARD',KEYS[1])<tonumber(ARGV[4]) then redis.call('SADD',KEYS[1],u) else redis.call('ZADD',KEYS[2],now,u) end
 end
 for _,u in ipairs(redis.call('ZRANGEBYSCORE',KEYS[2],'-inf',now,'LIMIT',0,200)) do
  if redis.call('SCARD',KEYS[1])>=tonumber(ARGV[4]) then break end
  redis.call('ZREM',KEYS[2],u); redis.call('SADD',KEYS[1],u)
 end
 local u=nil
 local best=nil
 for _,candidate in ipairs(redis.call('SMEMBERS',KEYS[1])) do
  local host=string.match(candidate,'^https?://([^/]+)') or candidate
  local available=tonumber(redis.call('HGET','crawler:host_ready',host) or '0')
  if available<=now then
   local score=tonumber(redis.call('HGET','crawler:host_last_claim',host) or '0')
   if best==nil or score<best or (score==best and candidate<u) then u=candidate; best=score end
  end
 end
 if not u then return '' end
 local host=string.match(u,'^https?://([^/]+)') or u
 redis.call('HSET','crawler:host_ready',host,now+600)
 redis.call('HSET','crawler:host_last_claim',host,now)
 redis.call('SREM',KEYS[1],u)
 redis.call('SREM','urls_to_crawl_priority',u)
 redis.call('ZADD',KEYS[3],now+600,u); redis.call('HSET',KEYS[4],u,ARGV[2])
 redis.call('HSET','crawler:last_attempt',u,now)
 return u`, []string{"urls_to_crawl", "crawler:scheduled", "crawler:inflight", "crawler:tokens"}, time.Now().Unix(), token, 0, maxQueueSize).Text()
	return v, token, err
}

// FinishClaim acknowledges only the current lease. A successful fetch records
// true freshness; retries and recrawls are scheduled separately.
func FinishClaim(u, token string, success bool, delay time.Duration, status ...int) error {
	code := 200
	if len(status) > 0 {
		code = status[0]
	}
	v, err := RedisClient.Eval(ctx, `
 if redis.call('HGET',KEYS[1],ARGV[1])~=ARGV[2] or tonumber(redis.call('ZSCORE',KEYS[2],ARGV[1]) or '0')<=tonumber(ARGV[4]) then return 0 end
 redis.call('HDEL',KEYS[1],ARGV[1]); redis.call('ZREM',KEYS[2],ARGV[1])
 local host=string.match(ARGV[1],'^https?://([^/]+)') or ARGV[1]
 local ready=tonumber(ARGV[4])+2
 if ARGV[3]=='0' then ready=tonumber(ARGV[5]) end
 redis.call('HSET','crawler:host_ready',host,ready)
 if ARGV[3]=='1' and ((tonumber(ARGV[6])>=200 and tonumber(ARGV[6])<300) or ARGV[6]=='304') then
  redis.call('SADD','urls_crawled',ARGV[1]); redis.call('ZADD','urls_crawled_at',ARGV[4],ARGV[1])
  redis.call('HSET','crawler:page:'..ARGV[1],'interval_seconds',tonumber(ARGV[5])-tonumber(ARGV[4]))
  redis.call('HDEL','crawler:attempts',ARGV[1]); redis.call('HINCRBY','crawler:stats','fetch_success_total',1)
 else redis.call('HINCRBY','crawler:stats','fetch_failure_total',1) end
 if ARGV[3]=='0' and tonumber(redis.call('HGET','crawler:attempts',ARGV[1]) or '0')>=10 then
 redis.call('ZADD','crawler:dead',ARGV[4],ARGV[1]); return 1 end
 redis.call('ZADD',KEYS[3],ARGV[5],ARGV[1]); return 1`, []string{"crawler:tokens", "crawler:inflight", "crawler:scheduled"}, u, token, boolInt(success), time.Now().Unix(), time.Now().Add(delay).Unix(), code).Int()
	if err == nil && v == 0 {
		return fmt.Errorf("expired or superseded lease")
	}
	return err
}
func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
func RetryDelay(u string) time.Duration {
	n, err := RedisClient.HIncrBy(ctx, "crawler:attempts", u, 1).Result()
	if err != nil {
		return time.Minute
	}
	if n > 10 {
		n = 10
	}
	return time.Duration(1<<uint(n)) * 30 * time.Second
}
func SaveFetchMetadata(u, etag, modified, hash string) error {
	return RedisClient.HSet(ctx, "crawler:page:"+u, map[string]interface{}{"etag": etag, "modified": modified, "hash": hash}).Err()
}
func FetchMetadata(u string) (map[string]string, error) {
	return RedisClient.HGetAll(ctx, "crawler:page:"+u).Result()
}

// RenewClaim extends only an unexpired lease owned by this worker.
func RenewClaim(u, token string) error {
	n, err := RedisClient.Eval(ctx, `
 if redis.call('HGET','crawler:tokens',ARGV[1])~=ARGV[2] or tonumber(redis.call('ZSCORE','crawler:inflight',ARGV[1]) or '0')<=tonumber(ARGV[3]) then return 0 end
 redis.call('ZADD','crawler:inflight',ARGV[4],ARGV[1])
 local host=string.match(ARGV[1],'^https?://([^/]+)') or ARGV[1]
 redis.call('HSET','crawler:host_ready',host,ARGV[4])
 return 1`, nil, u, token, time.Now().Unix(), time.Now().Add(10*time.Minute).Unix()).Int()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("expired or superseded lease")
	}
	return nil
}

// SaveClaimMetadata fences mutable page state, unlike idempotent graph merges.
func SaveClaimMetadata(u, token, etag, modified, hash string) error {
	n, err := RedisClient.Eval(ctx, `
 if redis.call('HGET','crawler:tokens',ARGV[1])~=ARGV[2] or tonumber(redis.call('ZSCORE','crawler:inflight',ARGV[1]) or '0')<=tonumber(ARGV[3]) then return 0 end
 redis.call('HSET',KEYS[1],'etag',ARGV[4],'modified',ARGV[5],'hash',ARGV[6])
 return 1`, []string{"crawler:page:" + u}, u, token, time.Now().Unix(), etag, modified, hash).Int()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("expired or superseded lease")
	}
	return nil
}

// RecordProcessingDuration exposes cumulative metrics (derive mean/rate externally).
func RecordProcessingDuration(d time.Duration) error {
	pipe := RedisClient.TxPipeline()
	pipe.HIncrBy(ctx, "crawler:stats", "processing_total", 1)
	pipe.HIncrBy(ctx, "crawler:stats", "processing_milliseconds_total", d.Milliseconds())
	_, err := pipe.Exec(ctx)
	return err
}
