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

local ttl = tonumber(ARGV[5])
if not ttl or ttl <= 0 then
  return 0
end

redis.call("HSET", KEYS[1],
  "user_id", ARGV[1],
  "refresh_id", ARGV[2],
  "expires_at", ARGV[3],
  "session_only", ARGV[4]
)
redis.call("PEXPIRE", KEYS[1], ttl)

redis.call("ZREMRANGEBYSCORE", KEYS[2], "-inf", ARGV[8])
redis.call("ZADD", KEYS[2], ARGV[3], ARGV[6])

local max = redis.call("ZREVRANGE", KEYS[2], 0, 0, "WITHSCORES")
redis.call("PEXPIRE", KEYS[2], tonumber(max[2]) * 1000 + tonumber(ARGV[7]) - tonumber(ARGV[9]))
return 1
