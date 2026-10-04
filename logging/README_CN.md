# logging

- `New` 返回 `*slog.Logger`
- `Helper` 包装 `*slog.Logger`
- 公共包边界使用 `*slog.Logger`
- 应用代码可使用 `Helper`

每次调用 `Helper` 日志方法和 `Enabled` 时传入非 nil context：

```go
logger := logging.NewHelper(logging.New(logging.Options{
	Level:     logging.LevelInfo,
	AddSource: true,
}))
logger = logger.With(logging.String("service", "accounts"))
logger.Info(ctx, "user saved", nil, logging.String("user_id", userID))
logger.Error(ctx, "save user failed", err, logging.String("user_id", userID))
```

请求处理使用请求 context，启动日志使用 `context.Background()`。
context 会传给底层 handler，用于过滤和输出。非 nil error 以 `error` 属性追加。
`With` 保存自己的属性切片副本。

`AddSource` 在文本和 JSON 输出中包含应用调用位置。
文本格式使用 `source=file:line`，JSON 格式使用 slog 的 source 对象。

文本格式会引用包含不可打印字符的消息，并转义不安全的属性键和值。
分组名作为完整属性键的一部分编码。
