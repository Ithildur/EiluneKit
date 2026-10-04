-- Create a session and maintain the per-user session index.
-- KEYS[1] = session key
-- KEYS[2] = user sessions key
-- ARGV[1] = user id
-- ARGV[2] = refresh id
-- ARGV[3] = expiration unix seconds
-- ARGV[4] = session only
-- ARGV[5] = session TTL in ms
-- ARGV[6] = session id
-- ARGV[7] = index TTL grace in ms
-- ARGV[8] = now unix seconds
-- ARGV[9] = now unix ms
-- ARGV[10] = max sessions per user, zero for unlimited
-- ARGV[11] = session key prefix

local ttl = tonumber(ARGV[5])
if not ttl or ttl <= 0 then
  return 0
end

redis.call("ZREMRANGEBYSCORE", KEYS[2], "-inf", ARGV[8])
local limit = tonumber(ARGV[10])
if limit > 0 and redis.call("ZCARD", KEYS[2]) >= limit then
  local replacing = redis.call("ZSCORE", KEYS[2], ARGV[6])
      and redis.call("HGET", KEYS[1], "user_id") == ARGV[1]
  if not replacing then
    -- Global cleanup can leave index members after their session records are gone.
    -- 全局清理可能留下对应会话记录已不存在的索引成员。
    local active = 0
    while active < limit do
      local ids = redis.call("ZRANGE", KEYS[2], active, limit - 1)
      if #ids == 0 then
        break
      end
      for _, id in ipairs(ids) do
        if redis.call("HGET", ARGV[11] .. id, "user_id") == ARGV[1] then
          active = active + 1
        else
          redis.call("ZREM", KEYS[2], id)
        end
      end
    end
    if active >= limit then
      return 2
    end
  end
end

redis.call("HSET", KEYS[1],
  "user_id", ARGV[1],
  "refresh_id", ARGV[2],
  "expires_at", ARGV[3],
  "session_only", ARGV[4]
)
redis.call("PEXPIRE", KEYS[1], ttl)

redis.call("ZADD", KEYS[2], ARGV[3], ARGV[6])

local max = redis.call("ZREVRANGE", KEYS[2], 0, 0, "WITHSCORES")
redis.call("PEXPIRE", KEYS[2], tonumber(max[2]) * 1000 + tonumber(ARGV[7]) - tonumber(ARGV[9]))
return 1
