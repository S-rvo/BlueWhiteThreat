package db

// crawler:scheduled remains the durable source of truth. The secondary index
// holds a ZSET per host and a small ZSET of host eligibility times. Validate
// the URL and due score against the source before every claim so a rollback
// or a mixed-version rollout cannot turn stale index data into duplicate work.
const scheduledHostsLua = `
local function scheduledHost(u)
 return redis.call('HGET','crawler:url_host',u) or string.match(u,'^https?://([^/?#]+)')
end
local function refreshScheduledHost(host)
 if not host then return end
 local head=redis.call('ZRANGE','crawler:scheduled_host:'..host,0,0,'WITHSCORES')
 if not head[1] then redis.call('ZREM','crawler:scheduled_hosts',host); return end
 local ready=tonumber(redis.call('HGET','crawler:host_ready',host) or '0')
 redis.call('ZADD','crawler:scheduled_hosts',math.max(ready,tonumber(head[2])),host)
end
local function scheduleURL(key,u,due)
 redis.call('ZADD',key,due,u)
 local host=scheduledHost(u)
 if host then
  redis.call('ZADD','crawler:scheduled_host:'..host,due,u)
  refreshScheduledHost(host)
 end
end
`
