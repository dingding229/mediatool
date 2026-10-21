package catalog

import (
	"context"
	"time"
)

const BrowseCacheTotalBytes = 16 << 20
const BrowseCacheEntries = 64

// Only this disposable cache namespace is evicted. Leases, session authority,
// receipts and all other Redis data are never deletion candidates, even if the
// cache index itself was poisoned. Expired payloads are reclaimed on each write.
const writeBrowseCache = `
local index=KEYS[2];local sizes=KEYS[3];local bytes=string.len(ARGV[1]);local limit=tonumber(ARGV[3]);local max=tonumber(ARGV[4])
if bytes>limit then return 0 end
local it=redis.call('TYPE',index).ok;local st=redis.call('TYPE',sizes).ok
if (it~='none' and it~='zset') or (st~='none' and st~='hash') then return 0 end
local function owned(k) return string.len(k)==81 and string.match(k,'^native:browse:v2:[0-9a-f]+$')~=nil end
local total=0
for _,k in ipairs(redis.call('ZRANGE',index,0,-1)) do
 if not owned(k) or redis.call('EXISTS',k)==0 then redis.call('ZREM',index,k);redis.call('HDEL',sizes,k)
 else total=total+(tonumber(redis.call('HGET',sizes,k)) or 0) end
end
if redis.call('ZSCORE',index,KEYS[1]) then
 total=total-(tonumber(redis.call('HGET',sizes,KEYS[1])) or 0);redis.call('ZREM',index,KEYS[1]);redis.call('HDEL',sizes,KEYS[1])
end
while total+bytes>limit or redis.call('ZCARD',index)>=max do
 local victim=redis.call('ZRANGE',index,0,0)[1];if not victim then break end
 total=total-(tonumber(redis.call('HGET',sizes,victim)) or 0)
 if owned(victim) then redis.call('DEL',victim) end
 redis.call('ZREM',index,victim);redis.call('HDEL',sizes,victim)
end
local now=redis.call('TIME');local score=tonumber(now[1])*1000+math.floor(tonumber(now[2])/1000)
redis.call('SET',KEYS[1],ARGV[1],'PX',ARGV[2]);redis.call('ZADD',index,score,KEYS[1]);redis.call('HSET',sizes,KEYS[1],bytes)
redis.call('PEXPIRE',index,tonumber(ARGV[2])*2);redis.call('PEXPIRE',sizes,tonumber(ARGV[2])*2)
return 1`

func (r *CachedRepository) store(ctx context.Context, key string, raw []byte) error {
	return r.redis.Eval(ctx, writeBrowseCache, []string{key, "native:browse:budget:v2:index", "native:browse:budget:v2:sizes"}, raw, int64(BrowseCacheTTL/time.Millisecond), BrowseCacheTotalBytes, BrowseCacheEntries).Err()
}
