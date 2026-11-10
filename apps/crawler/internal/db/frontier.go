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
	v, err := RedisClient.Eval(ctx, indexURLLua+`
 local now=tonumber(ARGV[1])
 for _,u in ipairs(redis.call('ZRANGEBYSCORE',KEYS[3],'-inf',now,'LIMIT',0,200)) do
  redis.call('ZREM',KEYS[3],u); redis.call('HDEL',KEYS[4],u)
  if redis.call('SCARD',KEYS[1])<tonumber(ARGV[4]) then redis.call('SADD',KEYS[1],u); indexURL(u,now) else redis.call('ZADD',KEYS[2],now,u) end
 end
 for _,u in ipairs(redis.call('ZRANGEBYSCORE',KEYS[2],'-inf',now,'LIMIT',0,200)) do
  if redis.call('SCARD',KEYS[1])>=tonumber(ARGV[4]) then break end
  local due=tonumber(redis.call('ZSCORE',KEYS[2],u))
  redis.call('HINCRBY','crawler:stats','scheduled_delay_seconds_total',math.max(0,now-due))
  redis.call('HINCRBY','crawler:stats','scheduled_promoted_total',1)
  redis.call('ZREM',KEYS[2],u); redis.call('SADD',KEYS[1],u); indexURL(u,now)
 end


 local u=nil
 for _,host in ipairs(redis.call('ZRANGEBYSCORE','crawler:hosts','-inf',now,'LIMIT',0,64)) do
  local available=tonumber(redis.call('HGET','crawler:host_ready',host) or '0')
  if available>now then redis.call('ZADD','crawler:hosts',available,host)
  else
   local q='crawler:host_queue:'..host
   for i=1,64 do
    local head=redis.call('ZRANGE',q,0,0)[1]
    if not head then redis.call('ZREM','crawler:hosts',host); break end
    redis.call('ZREM',q,head)
    if redis.call('SISMEMBER',KEYS[1],head)==1 then
     local canonical=redis.call('HGET','crawler:url_host',head) or host
     if canonical~=host then indexURL(head,now)
     else u=head;break end
    end
   end
  end
  if u then break end
 end
 if not u then return '' end
 local host=redis.call('HGET','crawler:url_host',u) or string.match(u,'^https?://([^/?#]+)')
 redis.call('HSET','crawler:host_ready',host,now+600)
 redis.call('HSET','crawler:host_last_claim',host,now)
 redis.call('ZADD','crawler:hosts',now+600,host)
 redis.call('SREM',KEYS[1],u)
 redis.call('SREM','urls_to_crawl_priority',u)
 redis.call('ZADD',KEYS[3],now+600,u); redis.call('HSET',KEYS[4],u,ARGV[2])
 redis.call('HSET','crawler:last_attempt',u,now)
 return u`, []string{"urls_to_crawl", "crawler:scheduled", "crawler:inflight", "crawler:tokens"}, time.Now().Unix(), token, 0, maxQueueSize).Text()
	return v, token, err
}

// FinishClaim acknowledges only the current lease. A successful fetch records
// true freshness; retries and recrawls are scheduled separately.
//
// Contrat public avec le scraper (ne pas rompre sans mettre à jour les deux
// côtés) : toute URL versée dans "urls_crawled" reçoit dans le même Lua
// atomique son horodatage de crawl dans le sorted set "urls_crawled_at"
// (SADD + ZADD indissociables). Le scraper consomme "urls_crawled_at" comme clé
// de fraîcheur pour prioriser les URLs les plus récemment crawlées (service
// .onion le plus susceptible d'être encore en vie). Invariant : aucun membre de
// "urls_crawled" ne doit exister sans score dans "urls_crawled_at".
func FinishClaim(u, token string, success bool, delay time.Duration, status ...int) error {
	host, err := HostKey(u)
	if err != nil {
		return err
	}
	code := 200
	if len(status) > 0 {
		code = status[0]
	}
	v, err := RedisClient.Eval(ctx, `
 if redis.call('HGET',KEYS[1],ARGV[1])~=ARGV[2] or tonumber(redis.call('ZSCORE',KEYS[2],ARGV[1]) or '0')<=tonumber(ARGV[4]) then return 0 end
 redis.call('HDEL',KEYS[1],ARGV[1]); redis.call('ZREM',KEYS[2],ARGV[1])
 local host=ARGV[7]
 local latency=tonumber(redis.call('HGET','crawler:host_stats:'..host,'latency_ewma_ms') or '0')
 local ready=tonumber(ARGV[4])+math.max(2,math.min(60,math.ceil(latency/1000)))
 if ARGV[3]=='0' then ready=tonumber(ARGV[5]) end
 redis.call('HSET','crawler:host_ready',host,ready)
 redis.call('ZADD','crawler:hosts',ready,host)
 if ARGV[3]=='1' and ((tonumber(ARGV[6])>=200 and tonumber(ARGV[6])<300) or ARGV[6]=='304') then
  redis.call('SADD','urls_crawled',ARGV[1]); redis.call('ZADD','urls_crawled_at',ARGV[4],ARGV[1])
  redis.call('HSET','crawler:page:'..ARGV[1],'interval_seconds',tonumber(ARGV[5])-tonumber(ARGV[4]))
  redis.call('HDEL','crawler:attempts',ARGV[1]); redis.call('HINCRBY','crawler:stats','fetch_success_total',1)
 else redis.call('HINCRBY','crawler:stats','fetch_failure_total',1) end
 if ARGV[3]=='0' and tonumber(redis.call('HGET','crawler:attempts',ARGV[1]) or '0')>=10 then
 redis.call('ZADD','crawler:dead',ARGV[4],ARGV[1]); return 1 end
 redis.call('ZADD',KEYS[3],ARGV[5],ARGV[1]); return 1`, []string{"crawler:tokens", "crawler:inflight", "crawler:scheduled"}, u, token, boolInt(success), time.Now().Unix(), time.Now().Add(delay).Unix(), code, host).Int()
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
	host, err := HostKey(u)
	if err != nil {
		return err
	}
	n, err := RedisClient.Eval(ctx, `
 if redis.call('HGET','crawler:tokens',ARGV[1])~=ARGV[2] or tonumber(redis.call('ZSCORE','crawler:inflight',ARGV[1]) or '0')<=tonumber(ARGV[3]) then return 0 end
 redis.call('ZADD','crawler:inflight',ARGV[4],ARGV[1])
 local host=ARGV[5]
 redis.call('HSET','crawler:host_ready',host,ARGV[4])
 redis.call('ZADD','crawler:hosts',ARGV[4],host)
 return 1`, nil, u, token, time.Now().Unix(), time.Now().Add(10*time.Minute).Unix(), host).Int()
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
 if ARGV[6]~='' then
  local key='crawler:content:'..ARGV[6]
  local old=redis.call('GET',key)
  if old and redis.call('HGET','crawler:page:'..old,'hash')~=ARGV[6] then redis.call('DEL',key) end
  redis.call('SET',key,ARGV[1],'EX',2592000,'NX')
  redis.call('HSET',KEYS[1],'content_id',ARGV[6])
  local canonical=redis.call('GET',key)
  redis.call('HSET',KEYS[1],'content_canonical',canonical)
  if canonical~=ARGV[1] then redis.call('HINCRBY','crawler:stats','duplicate_content_total',1) end
 end
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

func RecordHostFetch(raw string, status int, duration time.Duration, failed bool) error {
	host, err := HostKey(raw)
	if err != nil {
		return err
	}
	key := "crawler:host_stats:" + host
	p := RedisClient.TxPipeline()
	p.HIncrBy(ctx, key, "requests", 1)
	p.HIncrBy(ctx, key, "milliseconds", duration.Milliseconds())
	p.HIncrBy(ctx, key, fmt.Sprintf("status_%d", status), 1)
	if failed {
		p.HIncrBy(ctx, key, "errors", 1)
	}
	p.Eval(ctx, `local old=tonumber(redis.call('HGET',KEYS[1],'latency_ewma_ms') or ARGV[1]);redis.call('HSET',KEYS[1],'latency_ewma_ms',old*0.8+tonumber(ARGV[1])*0.2);return 1`, []string{key}, duration.Milliseconds())
	for _, bound := range []int64{100, 500, 1000, 5000, 15000, 45000} {
		if duration.Milliseconds() <= bound {
			p.HIncrBy(ctx, key, fmt.Sprintf("duration_le_%d_ms", bound), 1)
		}
	}
	p.Expire(ctx, key, 30*24*time.Hour)
	_, err = p.Exec(ctx)
	return err
}

// indexURLLua is shared by admission, promotion and legacy reconciliation.
const indexURLLua = `
local function indexURL(u,now)
 local host=redis.call('HGET','crawler:url_host',u) or string.match(u,'^https?://([^/?#]+)')
 redis.call('ZADD','crawler:host_queue:'..host,'NX',now,u)
 local ready=tonumber(redis.call('HGET','crawler:host_ready',host) or '0')
 redis.call('ZADD','crawler:hosts',ready,host)
end
`

// ReconcileFrontier is an explicit startup repair, never part of claiming work.
func ReconcileFrontier() error {
	// Register legacy URLs without deleting backlog. HSETNX makes concurrent
	// startup migrations idempotent and preserves admissions occurring meanwhile.
	for _, key := range []string{"urls_to_crawl", "urls_crawled", "crawler:scheduled", "crawler:inflight", "crawler:dead"} {
		var cursor uint64
		for {
			var values []string
			var next uint64
			var err error
			if key == "urls_to_crawl" || key == "urls_crawled" {
				values, next, err = RedisClient.SScan(ctx, key, cursor, "*", 200).Result()
			} else {
				var pairs []string
				pairs, next, err = RedisClient.ZScan(ctx, key, cursor, "*", 200).Result()
				for i := 0; i < len(pairs); i += 2 {
					values = append(values, pairs[i])
				}
			}
			if err != nil {
				return err
			}
			for _, u := range values {
				host, err := HostKey(u)
				if err != nil {
					return err
				}
				if err := RedisClient.Eval(ctx, indexURLLua+`
if redis.call('HSETNX','crawler:url_host',ARGV[1],ARGV[2])==1 then redis.call('HINCRBY','crawler:stored_per_host',ARGV[2],1) end
if redis.call('SISMEMBER','urls_to_crawl',ARGV[1])==1 then indexURL(ARGV[1],ARGV[3]) end return 1`, nil, u, host, time.Now().Unix()).Err(); err != nil {
					return err
				}
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
	}
	return nil
}
