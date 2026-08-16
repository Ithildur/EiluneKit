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

## 自动流水线

`NewClient` 返回底层的 `*redis.Client`。对于大量并发、短小且相互独立的命令，可通过该 client 显式启用 go-redis Automatic Pipelining：

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

默认的 blocking autopipeliner 保持普通同步命令形态，并使用有序 batch stream。多个 goroutine 的命令发生重叠时可以提高吞吐；单个 goroutine 的顺序命令仍会等待前一个结果。

Automatic Pipelining 在 go-redis v9.22.0 中仍是实验功能，并会改变两个重要的命令契约：

- 命令进入队列后会使用 autopipeliner 的长生命周期 context。必须遵守命令自身 deadline 或 cancellation 的操作应继续使用原始 `client`。
- 失败的 batch 可能整体重试。`INCR` 或修改状态的脚本等不得重复执行的非幂等命令应继续使用原始 `client`。

关闭 `client` 时也会关闭其共享的 autopipeliner。

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
