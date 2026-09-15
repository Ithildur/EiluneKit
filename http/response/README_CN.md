# http/response

[English](README.md)

JSON 写入函数和可选的错误 handler。导入路径：`github.com/Ithildur/EiluneKit/http/response`。

## 错误 handler

| 构造函数 | 状态码 | Code | Message |
|---|---|---|---|
| `NotFound()` | 404 | `not_found` | resource not found |
| `MethodNotAllowed()` | 405 | `method_not_allowed` | method not allowed |
| `Unauthorized()` | 401 | `unauthorized` | authentication required |
| `InternalServerError()` | 500 | `internal_error` | internal server error |

每个构造函数返回标准 `http.HandlerFunc`，写入 `ErrorResponse`，并设置 `Content-Type: application/json; charset=utf-8`。handler 不选择路径，也不会自动安装。路由器负责 `Allow`；认证方案相关的响应头由应用设置。

构建 API handler 时显式注入预设：

```go
handler, err := routes.NewHandler(api.RoutesAt("/api"), routes.HandlerOptions{
    NotFound:         response.NotFound(),
    MethodNotAllowed: response.MethodNotAllowed(),
    Unauthorized:     response.Unauthorized(),
    Middleware: []routes.Middleware{
        middleware.Recover(middleware.RecoverOptions{
            Logger:  logger,
            OnPanic: response.InternalServerError(),
        }),
    },
})
if err != nil {
    return err
}
```

这里的 `api` 是应用的 Blueprint，`logger` 是应用的 `*slog.Logger`。`NewHandler` 在调用 405 handler 前设置 `Allow`。`Recover` 只在响应开始前调用 `OnPanic`。注入预设不会改变其他 handler 的默认行为。

与 SPA 共用服务时，由应用选择哪些路径返回 JSON 错误，见 [SPA 兜底](../static/README_CN.md)。

## 覆盖预设

替换对应的注入项，其他项可以继续使用预设：

```go
func unauthorized(w http.ResponseWriter, r *http.Request) {
    response.WriteJSONError(w, http.StatusUnauthorized,
        "session_expired", "please sign in again")
}

handler, err := routes.NewHandler(api.RoutesAt("/api"), routes.HandlerOptions{
    NotFound:         response.NotFound(),
    MethodNotAllowed: response.MethodNotAllowed(),
    Unauthorized:     http.HandlerFunc(unauthorized),
})
if err != nil {
    return err
}
```

替换只作用于当前 handler 实例，不会重新定义 Kit 函数或修改全局配置。需要改变响应结构时，可通过 `WriteJSON` 写入应用自己的 struct。

## 新增业务错误 handler

定义普通 handler，再接入相应的应用边界。例如，将导出配额错误用作限流回调：

```go
func quotaExceeded(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Retry-After", "60")
    response.WriteJSONError(w, http.StatusTooManyRequests,
        "export_quota_exceeded", "try again in one minute")
}

exports := routes.NewBlueprint(routes.DefaultMiddleware(
    middleware.RateLimit(middleware.RateLimitOptions{
        Requests: 10,
        Window:   time.Minute,
        OnLimit:  quotaExceeded,
    }),
))
exports.Post("/exports", "Create export", createExport)
```

`createExport` 是应用的端点函数，无须向 response 包注册新错误。业务 handler 也可以直接调用 `quotaExceeded(w, r)` 后返回。

[可运行范例](example_test.go) 覆盖预设响应、自定义 401 和新增配额 handler。运行命令：`go test ./http/response -run Example -v`。
