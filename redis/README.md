# redis

Redis client helpers.

## Usage

```go
client, err := redis.NewClient(redis.Config{
	Addr: "localhost:6379",
})
if err != nil {
	return err
}
```

`NewClient` enables `ContextTimeoutEnabled`: network I/O observes the earlier of the context deadline and the configured socket timeout. The caller owns the client and closes it with `Close`.

## Session storage

`auth/store/redissession.Options.ReadTimeout` and `WriteTimeout` bound whole store operations, including calls that execute multiple Redis commands. Client socket timeouts apply to individual network reads and writes.

Clients created directly with `github.com/redis/go-redis/v9` (aliased as `goredis` below) must enable context deadlines before being passed to the session store:

```go
client := goredis.NewClient(&goredis.Options{
	Addr:                  "localhost:6379",
	ContextTimeoutEnabled: true,
})
defer client.Close()

store := redissession.New(client, redissession.Options{})
```

`ClearAllSessions` deletes session records and leaves stale index entries for `Sessions` to prune or the index TTL to expire. Creation also removes stale entries before rejecting a user at capacity. Concurrent logins may survive the scan and remain listed; cleanup does not provide an atomic cutoff. Until pruned, stale entries consume index space and add work to session listing.

## Automatic pipelining

`NewClient` returns the underlying `*redis.Client`. For workloads with many concurrent, small, independent commands, opt in to go-redis automatic pipelining through that client:

```go
client, err := redis.NewClient(redis.Config{
	Addr: "localhost:6379",
})
if err != nil {
	return err
}
defer client.Close()

commands, err := client.AutoPipeline()
if err != nil {
	return err
}

value, err := commands.Get(ctx, "key").Result()
```

The default blocking autopipeliner keeps the normal synchronous command shape and an ordered batch stream. It improves throughput when commands from multiple goroutines overlap; sequential commands from one goroutine still wait for each preceding result.

Automatic pipelining is experimental in go-redis v9.22.0 and changes two important command contracts:

- Once queued, a command runs on the autopipeliner's long-lived context. Use the original `client` when the command must honor its own deadline or cancellation.
- A failed batch may be retried as a whole. Use the original `client` for non-idempotent commands such as `INCR` or state-changing scripts that must not execute more than once.

Closing `client` also closes its shared autopipeliner.

## TLS

`Config.TLSConfig` is optional. Leave it nil for local Redis on loopback or another trusted private link.

Set `TLSConfig` when Redis is reached across an untrusted network or when a managed Redis provider requires TLS:

```go
client, err := redis.NewClient(redis.Config{
	Addr: "redis.example.com:6379",
	TLSConfig: &tls.Config{
		ServerName: "redis.example.com",
	},
})
```

## Notes

- `Addr` is required.
- `Ping` expects an explicit non-nil `context.Context`.
