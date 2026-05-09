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
