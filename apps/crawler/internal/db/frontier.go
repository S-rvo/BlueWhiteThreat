package db

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// ClaimURL atomically moves a ready task into a ten minute lease. Expired
// leases are recovered before selection; completion requires the lease token.
func ClaimURL(ctx context.Context) (string, string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", "", err
	}
	token := hex.EncodeToString(random[:])
	v, err := RedisClient.Eval(ctx, indexURLLua+scheduledHostsLua+`
 local now=tonumber(ARGV[1])
 for _,u in ipairs(redis.call('ZRANGEBYSCORE',KEYS[3],'-inf',now,'LIMIT',0,128)) do
  redis.call('ZREM',KEYS[3],u); redis.call('HDEL',KEYS[4],u)
  scheduleURL(KEYS[2],u,now)
 end
 -- Incremental migration/reconciliation. Use a shared cursor; don't scan a
 -- legacy 500k+ task ZSET all at once or during process initialization.
 if tonumber(redis.call('GET','crawler:scheduled_index_next') or '0')<=now then
  local cursor=redis.call('GET','crawler:scheduled_index_progress') or '0'
  local scan=redis.call('ZSCAN',KEYS[2],cursor,'COUNT',128)
  local changed={}
  for i=1,#scan[2],2 do
   local u,due=scan[2][i],scan[2][i+1]
   local host=scheduledHost(u)
   if host then
    redis.call('ZADD','crawler:scheduled_host:'..host,due,u)
    changed[host]=true
   end
  end
  for host,_ in pairs(changed) do refreshScheduledHost(host) end
  redis.call('SET','crawler:scheduled_index_progress',scan[1])
  if scan[1]=='0' then redis.call('SET','crawler:scheduled_index_next',now+1800) end
 end
 local function takeScheduled(u,host,due)
  redis.call('ZREM',KEYS[2],u)
  redis.call('ZREM','crawler:scheduled_host:'..host,u)
  refreshScheduledHost(host)
  redis.call('HINCRBY','crawler:stats','scheduled_delay_seconds_total',now-due)
  redis.call('HINCRBY','crawler:stats','scheduled_promoted_total',1)
  return u
 end
 local function scheduled()
  for _,host in ipairs(redis.call('ZRANGEBYSCORE','crawler:scheduled_hosts','-inf',now,'LIMIT',0,64)) do
   local q='crawler:scheduled_host:'..host
   local ready=tonumber(redis.call('HGET','crawler:host_ready',host) or '0')
   if ready<=now then
    for i=1,16 do
     local head=redis.call('ZRANGE',q,0,0,'WITHSCORES')
     local u=head[1]
     if not u then break end
     local due=redis.call('ZSCORE',KEYS[2],u)
     local canonical=scheduledHost(u)
     if not due or canonical~=host then
      redis.call('ZREM',q,u)
      if due and canonical then
       redis.call('ZADD','crawler:scheduled_host:'..canonical,due,u)
       refreshScheduledHost(canonical)
      end
     elseif tonumber(head[2])~=tonumber(due) then
      redis.call('ZADD',q,due,u)
     elseif tonumber(due)<=now then
      return takeScheduled(u,host,tonumber(due))
     else break end
    end
   end
   refreshScheduledHost(host)
  end
  -- Fallback while indexes are rebuilt or an older worker still writes.
  if redis.call('ZCOUNT',KEYS[2],'-inf',now)==0 then return nil end
  local cursor=redis.call('GET','crawler:scheduled_scan_cursor') or '0'
  local scan=redis.call('ZSCAN',KEYS[2],cursor,'COUNT',256)
  if scan[1]=='0' then redis.call('DEL','crawler:scheduled_scan_cursor')
  else redis.call('SET','crawler:scheduled_scan_cursor',scan[1]) end
  local page=scan[2]
  local best,bestDue,bestHost=nil,nil,nil
  for i=1,#page,2 do
   local candidate,due=page[i],tonumber(page[i+1])
   if due<=now and (not bestDue or due<bestDue or (due==bestDue and candidate<best)) then
    local host=scheduledHost(candidate)
    if host and tonumber(redis.call('HGET','crawler:host_ready',host) or '0')<=now then
     best,bestDue,bestHost=candidate,due,host
    end
   end
  end
  if best then return takeScheduled(best,bestHost,bestDue) end
  return nil
 end
 local function ready()
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
 return u
 end
 -- Alternate lanes when both have work; when a lane is blocked try the other.
 local fromScheduled=redis.call('HGET','crawler:stats','last_claim_scheduled')=='1'
 local u, deferred
 if fromScheduled then
  u=ready()
  if not u then u=scheduled(); deferred=true end
 else
  u=scheduled(); deferred=u~=nil
  if not u then u=ready() end
 end
 if not u then return '' end
 local host=scheduledHost(u)
 redis.call('HSET','crawler:stats','last_claim_scheduled',deferred and '1' or '0')
 redis.call('HSET','crawler:host_ready',host,now+600)
 refreshScheduledHost(host)
 redis.call('HSET','crawler:host_last_claim',host,now)
 redis.call('ZADD','crawler:hosts',now+600,host)
 redis.call('SREM',KEYS[1],u)
 redis.call('SREM','urls_to_crawl_priority',u)
 redis.call('ZADD',KEYS[3],now+600,u); redis.call('HSET',KEYS[4],u,ARGV[2])
 redis.call('HSET','crawler:last_attempt',u,now)
 return u`, []string{"urls_to_crawl", "crawler:scheduled", "crawler:inflight", "crawler:tokens"}, time.Now().Unix(), token).Text()
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
func FinishClaim(ctx context.Context, u, token string, success bool, delay time.Duration, status ...int) error {
	return finishClaim(ctx, u, token, success, false, delay, status...)
}

// DeferClaim reschedules a storage/infrastructure error, not a site failure.
func DeferClaim(ctx context.Context, u, token string, delay time.Duration) error {
	return finishClaim(ctx, u, token, false, true, delay)
}

func finishClaim(ctx context.Context, u, token string, success, infra bool, delay time.Duration, status ...int) error {
	host, err := HostKey(u)
	if err != nil {
		return err
	}
	code := 200
	if len(status) > 0 {
		code = status[0]
	}
	v, err := RedisClient.Eval(ctx, scheduledHostsLua+`
 if redis.call('HGET',KEYS[1],ARGV[1])~=ARGV[2] or tonumber(redis.call('ZSCORE',KEYS[2],ARGV[1]) or '0')<=tonumber(ARGV[4]) then return 0 end
 redis.call('HDEL',KEYS[1],ARGV[1]); redis.call('ZREM',KEYS[2],ARGV[1])
 local host=ARGV[7]
 local latency=tonumber(redis.call('HGET','crawler:host_stats:'..host,'latency_ewma_ms') or '0')
 local ready=tonumber(ARGV[4])+math.max(2,math.min(60,math.ceil(latency/1000)))
 if ARGV[3]=='0' and ARGV[8]=='0' then ready=tonumber(ARGV[5]) end
 redis.call('HSET','crawler:host_ready',host,ready)
 refreshScheduledHost(host)
 redis.call('ZADD','crawler:hosts',ready,host)
 if ARGV[8]=='1' then
  redis.call('HINCRBY','crawler:stats','infrastructure_retry_total',1)
  scheduleURL(KEYS[3],ARGV[1],ARGV[5]); return 1
 end
 if ARGV[6]=='410' then
  redis.call('HDEL','crawler:attempts',ARGV[1])
  redis.call('ZADD','crawler:gone',ARGV[4],ARGV[1])
  redis.call('HINCRBY','crawler:stats','gone_total',1); return 1
 end
 if ARGV[3]=='1' and ((tonumber(ARGV[6])>=200 and tonumber(ARGV[6])<300) or ARGV[6]=='304') then
  redis.call('SADD','urls_crawled',ARGV[1]); redis.call('ZADD','urls_crawled_at',ARGV[4],ARGV[1])
  if redis.call('SISMEMBER','urls_scraped',ARGV[1])==0 and not redis.call('ZSCORE','scraper:inflight',ARGV[1]) then
   redis.call('ZADD','scraper:pending',tonumber(ARGV[9])+tonumber(ARGV[4]),ARGV[1])
  end
  redis.call('HSET','crawler:page:'..ARGV[1],'interval_seconds',tonumber(ARGV[5])-tonumber(ARGV[4]))
  redis.call('EXPIRE','crawler:page:'..ARGV[1],7776000)
  redis.call('HDEL','crawler:attempts',ARGV[1]); redis.call('HINCRBY','crawler:stats','fetch_success_total',1)
 else redis.call('HINCRBY','crawler:stats','fetch_failure_total',1) end
 if ARGV[3]=='0' and redis.call('HINCRBY','crawler:attempts',ARGV[1],1)>=10 then
 redis.call('ZADD','crawler:dead',ARGV[4],ARGV[1]); return 1 end
 scheduleURL(KEYS[3],ARGV[1],ARGV[5]); return 1`, []string{"crawler:tokens", "crawler:inflight", "crawler:scheduled"}, u, token, boolInt(success), time.Now().Unix(), time.Now().Add(delay).Unix(), code, host, boolInt(infra), int64(queuePriority(u))*10_000_000_000).Int()
	if err == nil && v == 0 {
		return fmt.Errorf("expired or superseded lease")
	}
	return err
}

// DropClaim acknowledges the current lease without rescheduling: the URL is
// moved to the blocked dead-letter ("crawler:blocked") instead of being
// recycled through "crawler:scheduled". Use this for URLs rejected by policy
// (CSAM/farm), as opposed to FinishClaim's transient-failure retry path.
func DropClaim(ctx context.Context, u, token string) error {
	host, err := HostKey(u)
	if err != nil {
		return err
	}
	v, err := RedisClient.Eval(ctx, scheduledHostsLua+`
 if redis.call('HGET',KEYS[1],ARGV[1])~=ARGV[2] or tonumber(redis.call('ZSCORE',KEYS[2],ARGV[1]) or '0')<=tonumber(ARGV[3]) then return 0 end
 redis.call('HDEL',KEYS[1],ARGV[1]); redis.call('ZREM',KEYS[2],ARGV[1])
 local host=ARGV[4]
 redis.call('HSET','crawler:host_ready',host,ARGV[3])
 refreshScheduledHost(host)
 redis.call('ZADD','crawler:hosts',ARGV[3],host)
 redis.call('ZADD','crawler:blocked',ARGV[3],ARGV[1])
 redis.call('HDEL','crawler:attempts',ARGV[1])
 redis.call('HINCRBY','crawler:stats','blocked_dropped_total',1)
 return 1`, []string{"crawler:tokens", "crawler:inflight"}, u, token, time.Now().Unix(), host).Int()
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
func RetryDelay(ctx context.Context, u string) time.Duration {
	n, err := RedisClient.HGet(ctx, "crawler:attempts", u).Int64()
	if err != nil {
		return time.Minute
	}
	n = max(1, min(n+1, 10))
	return time.Duration(1<<n) * 30 * time.Second
}
func FetchMetadata(ctx context.Context, u string) (map[string]string, error) {
	return RedisClient.HGetAll(ctx, "crawler:page:"+u).Result()
}

// RenewClaim extends only an unexpired lease owned by this worker.
func RenewClaim(ctx context.Context, u, token string) error {
	host, err := HostKey(u)
	if err != nil {
		return err
	}
	n, err := RedisClient.Eval(ctx, scheduledHostsLua+`
 if redis.call('HGET','crawler:tokens',ARGV[1])~=ARGV[2] or tonumber(redis.call('ZSCORE','crawler:inflight',ARGV[1]) or '0')<=tonumber(ARGV[3]) then return 0 end
 redis.call('ZADD','crawler:inflight',ARGV[4],ARGV[1])
 local host=ARGV[5]
 redis.call('HSET','crawler:host_ready',host,ARGV[4])
 refreshScheduledHost(host)
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
func SaveClaimMetadata(ctx context.Context, u, token, etag, modified, hash string) error {
	n, err := RedisClient.Eval(ctx, `
 if redis.call('HGET','crawler:tokens',ARGV[1])~=ARGV[2] or tonumber(redis.call('ZSCORE','crawler:inflight',ARGV[1]) or '0')<=tonumber(ARGV[3]) then return 0 end

 redis.call('HSET',KEYS[1],'etag',ARGV[4],'modified',ARGV[5],'hash',ARGV[6])
 redis.call('EXPIRE',KEYS[1],7776000)
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
func RecordProcessingDuration(ctx context.Context, d time.Duration) error {
	pipe := RedisClient.TxPipeline()
	pipe.HIncrBy(ctx, "crawler:stats", "processing_total", 1)
	pipe.HIncrBy(ctx, "crawler:stats", "processing_milliseconds_total", d.Milliseconds())
	_, err := pipe.Exec(ctx)
	return err
}

func RecordHostFetch(ctx context.Context, raw string, status int, duration time.Duration, failed bool) error {
	host, err := HostKey(raw)
	if err != nil {
		return err
	}
	key := "crawler:host_stats:" + host
	p := RedisClient.TxPipeline()
	p.HIncrBy(ctx, key, "requests", 1)
	p.HIncrBy(ctx, key, "milliseconds", duration.Milliseconds())
	if status != 0 {
		p.HIncrBy(ctx, key, fmt.Sprintf("status_%d", status), 1)
	}
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
			_, err = RedisClient.Pipelined(ctx, func(pipe redis.Pipeliner) error {
				now := time.Now().Unix()
				for _, u := range values {
					host, err := HostKey(u)
					if err != nil {
						log.Printf("ReconcileFrontier: skipping unroutable legacy entry: %v", err)
						continue
					}
					pipe.Eval(ctx, indexURLLua+`
if redis.call('HSETNX','crawler:url_host',ARGV[1],ARGV[2])==1 then redis.call('HINCRBY','crawler:stored_per_host',ARGV[2],1) end
if redis.call('SISMEMBER','urls_to_crawl',ARGV[1])==1 then indexURL(ARGV[1],ARGV[3]) end return 1`, nil, u, host, now)
				}
				return nil
			})
			if err != nil {
				return err
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
	}
	return nil
}
