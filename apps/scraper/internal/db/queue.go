package db

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/pkg/contract"
	"github.com/redis/go-redis/v9"
)

const (
	scrapePending     = contract.KeyScraperPending
	scrapeInflight    = contract.KeyScraperInflight
	scrapeTokens      = "scraper:tokens"
	scrapeClaimScore  = "scraper:claim_scores"
	scrapeScanCursor  = "scraper:index_cursor"
	scrapeLeaseLength = 5 * time.Minute
)

type ScrapeClaim struct{ URL, Token string }

type ScrapeOutcome string

const (
	ScrapeSuccess        ScrapeOutcome = "success"
	ScrapeTransient      ScrapeOutcome = "transient"
	ScrapePermanent      ScrapeOutcome = "permanent"
	ScrapeBlocked        ScrapeOutcome = "blocked"
	ScrapeInfrastructure ScrapeOutcome = "infrastructure"
	ScrapeDeferred       ScrapeOutcome = "deferred"
)

func ScrapeBatchSize() (int, error) {
	raw := os.Getenv("SCRAPER_BATCH_SIZE")
	if raw == "" {
		return 300, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 5000 {
		return 0, fmt.Errorf("invalid SCRAPER_BATCH_SIZE %q (expected 1..5000)", raw)
	}
	return n, nil
}

// IndexScrapePending migrates existing members incrementally, also covering
// crawl events from an older crawler during a rolling update. No key is reset.
func IndexScrapePending(ctx context.Context, client *redis.Client, pages int) error {
	for page := 0; page < pages; page++ {
		old, err := client.Get(ctx, scrapeScanCursor).Uint64()
		if err != nil && err != redis.Nil {
			return err
		}
		urls, next, err := client.SScan(ctx, contract.KeyURLsCrawled, old, "", 512).Result()
		if err != nil {
			return err
		}
		args := make([]interface{}, 0, 2+2*len(urls))
		args = append(args, old, next)
		for _, u := range urls {
			args = append(args, u, contract.PendingPriorityBase(u))
		}
		err = indexScrapeScript.Run(ctx, client, []string{scrapeScanCursor, scrapePending, scrapeInflight}, args...).Err()
		if err != nil || next == 0 {
			return err
		}
	}
	return nil
}

var indexScrapeScript = redis.NewScript(`
if (redis.call('GET',KEYS[1]) or '0')~=ARGV[1] then return 0 end
for i=3,#ARGV,2 do
 local u=ARGV[i]
 if redis.call('SISMEMBER','urls_crawled',u)==1 and redis.call('SISMEMBER','urls_scraped',u)==0 and not redis.call('ZSCORE',KEYS[3],u)
    and not redis.call('ZSCORE','crawler:gone',u) and not redis.call('ZSCORE','crawler:blocked',u) then
  local freshness=tonumber(redis.call('ZSCORE','urls_crawled_at',u) or '0')
  redis.call('ZADD',KEYS[2],tonumber(ARGV[i+1])+freshness,u)
 end
end
redis.call('SET',KEYS[1],ARGV[2])
return 1
`)

func ClaimScrape(ctx context.Context, client *redis.Client) (ScrapeClaim, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return ScrapeClaim{}, err
	}
	now := time.Now()
	token := hex.EncodeToString(id[:])
	u, err := claimScrapeScript.Run(ctx, client, []string{scrapePending, scrapeInflight, scrapeTokens, scrapeClaimScore},
		now.Unix(), now.Add(scrapeLeaseLength).Unix(), token).Text()
	return ScrapeClaim{URL: u, Token: token}, err
}

var claimScrapeScript = redis.NewScript(`
local now = tonumber(ARGV[1])
for _,u in ipairs(redis.call('ZRANGEBYSCORE',KEYS[2],'-inf',now,'LIMIT',0,64)) do
 local score=redis.call('HGET',KEYS[4],u)
 redis.call('ZREM',KEYS[2],u)
 redis.call('HDEL',KEYS[3],u)
 redis.call('HDEL',KEYS[4],u)
 if score then
  redis.call('ZADD',KEYS[1],'GT',score,u)
  redis.call('HINCRBY','scraper:stats','recovered',1)
 end
end
-- Reserve one claim in four for lower-priority/older work when queued.
local take='ZPOPMAX'
if tonumber(redis.call('HGET','scraper:stats','claimed') or '0')%4==3 then take='ZPOPMIN' end
for i=1,64 do
 local row=redis.call(take,KEYS[1])
 if not row[1] then return '' end
 local u=row[1]
 if redis.call('SISMEMBER','urls_crawled',u)==1 and redis.call('SISMEMBER','urls_scraped',u)==0 and not redis.call('ZSCORE',KEYS[2],u)
    and not redis.call('ZSCORE','crawler:gone',u) and not redis.call('ZSCORE','crawler:blocked',u) then
  redis.call('ZADD',KEYS[2],ARGV[2],u)
  redis.call('HSET',KEYS[3],u,ARGV[3])
  redis.call('HSET',KEYS[4],u,row[2])
  redis.call('HINCRBY','scraper:stats','claimed',1)
  return u
 end
end
return ''
`)

// ReleaseScrape returns a lease on graceful shutdown without consuming a retry.
func ReleaseScrape(ctx context.Context, client *redis.Client, claim ScrapeClaim) error {
	return releaseScrapeScript.Run(ctx, client, []string{scrapePending, scrapeInflight, scrapeTokens, scrapeClaimScore},
		claim.URL, claim.Token, time.Now().Unix()).Err()
}

var releaseScrapeScript = redis.NewScript(`
local u=ARGV[1]
if redis.call('HGET',KEYS[3],u)~=ARGV[2] or tonumber(redis.call('ZSCORE',KEYS[2],u) or '0')<=tonumber(ARGV[3]) then return 0 end
local score=redis.call('HGET',KEYS[4],u)
redis.call('ZREM',KEYS[2],u)
redis.call('HDEL',KEYS[3],u)
redis.call('HDEL',KEYS[4],u)
if score then redis.call('ZADD',KEYS[1],'GT',score,u) end
redis.call('HINCRBY','scraper:stats','released',1)
return 1
`)

// FinishScrape acknowledges a result only if this worker still owns the lease.
func FinishScrape(ctx context.Context, client *redis.Client, claim ScrapeClaim, outcome ScrapeOutcome) error {
	switch outcome {
	case ScrapeSuccess, ScrapeTransient, ScrapePermanent, ScrapeBlocked, ScrapeInfrastructure, ScrapeDeferred:
	default:
		return fmt.Errorf("invalid scrape outcome %q", outcome)
	}
	n, err := finishScrapeScript.Run(ctx, client, []string{scrapePending, scrapeInflight, scrapeTokens, scrapeClaimScore},
		claim.URL, claim.Token, time.Now().Unix(), string(outcome)).Int()
	if err == nil && n != 1 {
		return fmt.Errorf("expired or superseded scrape lease")
	}
	return err
}

var finishScrapeScript = redis.NewScript(`
local u, token, now, outcome=ARGV[1],ARGV[2],tonumber(ARGV[3]),ARGV[4]
if redis.call('HGET',KEYS[3],u)~=token or tonumber(redis.call('ZSCORE',KEYS[2],u) or '0')<=now then return 0 end
redis.call('ZREM',KEYS[2],u)
redis.call('HDEL',KEYS[3],u)
redis.call('HDEL',KEYS[4],u)
redis.call('SADD','urls_scraped',u)
redis.call('HINCRBY','scraper:stats',outcome,1)
redis.call('ZREM','urls_scraped_at',u)
redis.call('ZREM','urls_scrape_failed_at',u)
if outcome=='deferred' then
 redis.call('ZADD','scraper:retry_due',now+60,u)
elseif outcome=='infrastructure' then
 redis.call('ZADD','urls_scrape_failed_at',now,u)
 redis.call('ZADD','scraper:retry_due',now+300,u)
elseif outcome=='transient' then
 local n=redis.call('HINCRBY','scraper:attempts',u,1)
 if n>=6 then
  -- Exhaustion is a quarantine, not proof that the remote URL is gone.
  redis.call('ZADD','scraper:dead',now,u)
 else
  redis.call('ZADD','urls_scrape_failed_at',now,u)
  redis.call('ZADD','scraper:retry_due',now+7200*2^(n-1),u)
 end
else
 redis.call('HDEL','scraper:attempts',u)
 if outcome=='success' then redis.call('ZADD','urls_scraped_at',now,u)
 elseif outcome=='blocked' then redis.call('ZADD','scraper:blocked',now,u)
 else redis.call('ZADD','scraper:invalid',now,u) end
end
return 1
`)
