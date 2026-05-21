-- Revoke a session and maintain the per-user session index.
-- KEYS[1] = session key
-- KEYS[2] = user sessions key
-- ARGV[1] = session id
-- ARGV[2] = index TTL grace in ms
-- ARGV[3] = now unix seconds
-- ARGV[4] = now unix ms

redis.call("DEL", KEYS[1])
redis.call("ZREM", KEYS[2], ARGV[1])
redis.call("ZREMRANGEBYSCORE", KEYS[2], "-inf", ARGV[3])

if redis.call("ZCARD", KEYS[2]) == 0 then
  redis.call("DEL", KEYS[2])
  return 1
end

local max = redis.call("ZREVRANGE", KEYS[2], 0, 0, "WITHSCORES")
redis.call("PEXPIRE", KEYS[2], tonumber(max[2]) * 1000 + tonumber(ARGV[2]) - tonumber(ARGV[4]))
return 1
