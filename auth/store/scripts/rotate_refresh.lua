-- Rotate session refresh state atomically.
-- KEYS[1] = user version key
-- KEYS[2] = session key
-- KEYS[3] = user sessions key
-- ARGV[1] = expected user version
-- ARGV[2] = expected user id
-- ARGV[3] = old refresh id
-- ARGV[4] = new refresh id
-- ARGV[5] = new expiration unix seconds
-- ARGV[6] = new TTL in ms
-- ARGV[7] = session id

local version = redis.call("GET", KEYS[1])
if not version then
  version = "0"
end
if tostring(version) ~= tostring(ARGV[1]) then
  return 0
end

local sessionUser = redis.call("HGET", KEYS[2], "user_id")
local currentRefresh = redis.call("HGET", KEYS[2], "refresh_id")
if not sessionUser or sessionUser ~= ARGV[2] then
  return 0
end
if not currentRefresh or currentRefresh ~= ARGV[3] then
  return 0
end

local ttl = tonumber(ARGV[6])
if not ttl or ttl <= 0 then
  return 0
end

redis.call("HSET", KEYS[2],
  "user_id", ARGV[2],
  "refresh_id", ARGV[4],
  "expires_at", ARGV[5]
)
redis.call("PEXPIRE", KEYS[2], ttl)
redis.call("ZADD", KEYS[3], ARGV[5], ARGV[7])
return 1
