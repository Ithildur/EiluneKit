# redis

Redis client 辅助包。

## 用法

```go
client, err := redis.NewClient(redis.Config{
	Addr: "localhost:6379",
})
if err != nil {
	return err
}
```

## TLS

`Config.TLSConfig` 是可选项。本机回环地址或可信私有链路上的 Redis 保持 nil 即可。

Redis 经过不可信网络访问，或托管 Redis 服务要求 TLS 时，设置 `TLSConfig`：

```go
client, err := redis.NewClient(redis.Config{
	Addr: "redis.example.com:6379",
	TLSConfig: &tls.Config{
		ServerName: "redis.example.com",
	},
})
```

## 说明

- `Addr` 必填。
- `Ping` 需要显式提供非空 `context.Context`。
