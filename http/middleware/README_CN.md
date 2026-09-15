# http/middleware

[English](README.md)

可选的标准 `func(http.Handler) http.Handler` 中间件。可以注入 `routes.HandlerOptions.Middleware`，通过 Blueprint 为端点附加策略，或配合任意 `net/http` 路由器使用。构造函数不会自动安装中间件。

## 应用中间件

```go
handler, err := routes.NewHandler(root.Routes(), routes.HandlerOptions{
	Middleware: []routes.Middleware{
		middleware.RequestID,
		middleware.AccessLog(middleware.AccessLogOptions{Logger: logger}),
		middleware.CORS(middleware.CORSOptions{
			AllowedOrigins:   []string{"https://app.example.com"},
			AllowedMethods:   []string{"GET", "POST", "DELETE"},
			AllowedHeaders:   []string{"Authorization", "Content-Type"},
			AllowCredentials: true,
			MaxAge:           5 * time.Minute,
		}),
		middleware.Compress(middleware.CompressOptions{MinSize: 500}),
		middleware.Recover(middleware.RecoverOptions{Logger: logger}),
	},
})
if err != nil {
	return err
}
```

列表从上向下进入，反向退出。请求 ID 位于日志之前；恢复中间件位于压缩内部，使 panic 响应使用相同的编码器。外层中间件自身的 panic 不在内层 `Recover` 的捕获范围内。CORS 预检可能在进入压缩、恢复或端点之前结束；必须覆盖预检的策略放在 CORS 之前。

## 请求 ID 与日志

`RequestID` 复用 chi 的请求 ID 中间件。接受传入的 `X-Request-Id`，或 chi 配置的请求 ID 头；没有时生成 ID。只设置 context，不设置响应头。使用 `RequestIDFromContext(ctx)` 读取；不存在或 context 为 nil 时返回空字符串。传入的 ID 只用于关联记录，不是可信身份。

`AccessLog` 使用同一 context，通过 `slog` 记录状态码、耗时毫秒数、方法、路径和客户端信息。Logger 为 nil 时禁用访问日志。`Skip` 和 `MinLevel` 控制输出；使用转发头中的客户端地址需要配置可信代理。

handler 因 panic 退出时也会记录访问日志，包括 `http.ErrAbortHandler`。这类记录使用 error 级别，包含 `aborted: true`；状态码为观察到的最终状态，未观察到最终状态时为 `0`，不会因 panic 虚构 500 或 200。普通响应保留原有字段和级别。`Skip` 和 `MinLevel` 同样适用于中止记录，因此 `Skip` 可能收到状态 `0`。panic 原样传播。

## Panic 恢复

`Recover(RecoverOptions{Logger: logger, OnPanic: failureHandler})` 记录 panic 值和调用栈。Logger 为 nil 时采用构造时的 `slog.Default()`。`OnPanic` 为 nil 时返回纯文本 500，不包含 panic 详情；自定义 handler 自行决定响应状态码和响应体。

设置 `OnPanic: response.InternalServerError()` 可返回 JSON 500。预设和自定义 handler 范例见 [http/response](../response/README_CN.md)。

调用 `OnPanic` 前会清除待发送的 `Content-Length`，允许失败响应体使用不同长度；其他响应头仍保留给 handler。

只捕获处理请求的 goroutine 中的 panic。响应开始后，会再次抛出 `http.ErrAbortHandler` 中止请求，不追加错误响应体。panic 时关闭已接管的连接；遇到 `http.ErrAbortHandler` 则直接抛出，不记录日志。包装器保留刷新、连接接管和 response controller 能力。恢复中间件不缓存响应。

## CORS

`CORS(CORSOptions{...})` 使用 `rs/cors`，在路由匹配前处理预检，也覆盖未注册路径。普通响应及错误响应都会按策略添加许可响应头。未获允许的来源不会获得许可响应头；CORS 不认证请求，也不阻止非浏览器客户端访问 handler。

| 配置 | 默认值与行为 |
|---|---|
| `AllowedOrigins` | 空列表不允许任何来源。支持精确来源及含一个 `*` 的模式；显式 `"*"` 允许所有来源。 |
| `AllowedMethods` | GET、HEAD、POST；请显式列出方法。 |
| `AllowedHeaders` | Accept、Content-Type、X-Requested-With；`"*"` 允许所有请求头。 |
| `ExposedHeaders` | 默认不额外暴露响应头。 |
| `AllowCredentials` | 默认 false；设为 true 并配合 `"*"` 来源时，构造阶段 panic。 |
| `MaxAge` | 零值保留浏览器预检缓存默认值；正值截断为整秒，负值 panic。 |

配置切片在构造时复制。不会安装动态配置注册表或全局 CORS 策略。

## 压缩

`Compress(CompressOptions{...})` 使用 `klauspost/compress/gzhttp` 压缩 gzip 响应。`Level` 接受 -2、-1 或 1–9；零值选择默认压缩级别。`MinSize` 的单位是字节，零值选择 1024。负数大小和无效级别在构造阶段 panic。

`ContentTypes` 可以将压缩限制为显式列出的 MIME 类型；为空时采用 gzhttp 的默认过滤器，排除常见已压缩的音频、视频和归档格式。无效 MIME 类型和通配符会 panic。支持参数：`text/plain` 也匹配 `text/plain; charset=utf-8`。

压缩遵守 gzip 协商，跳过 HEAD、已编码响应及内容范围响应，并添加 `Vary: Accept-Encoding`。压缩时移除原始 `ETag` 和 `Content-Length`。不启用 Zstd 或请求解压。只保留达到阈值或内容检测所需的初始缓冲，不缓存整个响应；支持刷新和连接接管。

## 其他中间件

- `LimitBody(maxBytes)` 在上限为正数时使用 `http.MaxBytesReader` 包装请求体，不预读，也不自动返回 413；错误由读取请求体的代码处理。
- `RequireJSONBody` 要求非空请求体使用 `application/json`。接受合法的媒体类型参数和空白；参数格式错误时返回 415。放在解码 JSON 的端点上。
- `RateLimit(RateLimitOptions{...})` 接受应用提供的 key 函数和 `OnLimit` 响应回调；IP key 支持配置可信代理。

API 前缀、缓存策略、请求体上限和错误格式由应用决定。通过 `routes.HandlerOptions` 注入失败响应；`routes.NewHandler` 会在调用应用的 405 handler 前设置 `Allow`。边界注入和 SPA 兜底见[完整 HTTP handler](../routes/README_CN.md#完整-http-handler)。
