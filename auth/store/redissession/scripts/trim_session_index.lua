-- Trim stale user session index members and maintain the index TTL.
-- KEYS[1] = user sessions key
-- ARGV[1] = index TTL grace in ms
-- ARGV[2] = now unix seconds
-- ARGV[3] = now unix ms
-- ARGV[4..n] = stale session ids

redis.call("ZREMRANGEBYSCORE", KEYS[1], "-inf", ARGV[2])

for i = 4, #ARGV do
  redis.call("ZREM", KEYS[1], ARGV[i])
end

if redis.call("ZCARD", KEYS[1]) == 0 then
  redis.call("DEL", KEYS[1])
  return 1
end

local max = redis.call("ZREVRANGE", KEYS[1], 0, 0, "WITHSCORES")
redis.call("PEXPIRE", KEYS[1], tonumber(max[2]) * 1000 + tonumber(ARGV[1]) - tonumber(ARGV[3]))
return 1
