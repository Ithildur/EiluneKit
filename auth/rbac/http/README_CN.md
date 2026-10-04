# auth/rbac/http

`auth/rbac/http` 将 `auth/rbac.Service` 适配为 JSON bearer 路由。

它用于需要多用户、角色检查、scope 检查或 opaque API token 的应用。认证流程由 `auth/rbac` 负责；用户存储、密码 hash、角色分配和 token 持久化由应用负责。

默认 HTTP 路径：

| 路由 | 用途 |
|---|---|
| `POST /auth/login` | 返回 `access_token`、`refresh_token` 和 `user` |
| `POST /auth/refresh` | 从 JSON body 轮换 `refresh_token` |
| `POST /auth/logout` | 从 JSON body 吊销 `refresh_token` |
| `GET /auth/me` | 返回当前主体 |

`POST /auth/login` 接收 `username`、`password` 和可选的 `persistence`。缺省 `persistence` 表示持久 token；`session` 返回 session-only token 元数据。

挂载与契约生成应使用同一组路由值：

```go
routeList := authHandler.Routes()
err := routes.Mount(r, "", routeList)

spec, err := openapi.Generate(routeList, openapi.Options{
	Title:   "Application RBAC API",
	Version: "1.0.0",
})
```

`Options.BasePath` 是 `*string` 类型的路由前缀：`nil` 默认使用 `"/auth"`，`new("")` 选择根目录，`new("/api/auth")` 选择 `/api/auth`。非空前缀必须以单个斜线开头；尾斜线和首尾空白会被拒绝。

## 推荐组合

登录入口默认按客户端 IP 前缀（IPv4 /24、IPv6 /40）每分钟允许五次请求。`Options.RateLimit` 复用 `auth/http` 的配置；外层限流器负责此策略时，可设置 `Disabled`。限流配置未覆盖的可信代理和客户端 IP 头从 handler 继承。刷新和其他端点不使用此登录限流器。

凭据错误、锁定和容量拒绝均返回相同的 `401 unauthorized`，消息为 `invalid credentials`，不附带可区分的响应头。独立限流返回 `429 rate_limited`。后端故障保持正常错误响应。耗时不做统一处理，`RejectNewKeys` 在表满时可能拒绝正确凭据。容量策略见 [Service 配置](../README_CN.md)。

登录锁定使用客户端 IP 和提交的用户名 hash。通过 `Options.TrustedProxies` 接受转发的客户端 IP，通过 `Options.ClientIPHeaders` 选择头的优先级。Nil 依次使用 `X-Forwarded-For`、`X-Real-IP`、`Forwarded`、`True-Client-IP`、`CF-Connecting-IP`；`[]string{}` 禁用客户端 IP 转发头。非空列表替换默认值。仅列出可信代理清理或设置的头。这些配置切片在构造 handler 时复制。

- 使用应用自己的 `UserStore` 和 `PasswordVerifier` 构造 `auth/rbac.Service`
- 使用 `auth/jwt.Manager`，多实例 session 状态用 `auth/store/redissession`
- 使用 `auth/rbac.Lockout`；`auth/rbac.Service` 默认使用内存锁定，多实例部署应传入 Redis 或 SQL 实现
- 需要 API token 走同一套 Bearer middleware 时，实现 `auth/rbac.APITokenStore`
- 有角色层级时传入 `Options.RolePolicy`；简单项目可使用默认精确角色匹配

可以从 `Handler.Middleware()` 或 `NewMiddleware` 获取中间件：

```go
authz := authHandler.Middleware()
r.Use(authz.RequireAuth())
r.With(authz.RequireRole("admin")).Get("/admin", adminHandler)
r.With(authz.RequireScope("vm:read")).Get("/vms", listVMs)
```

通过 `Options.RolePolicy` 或 `NewMiddleware(service, policy)` 配置角色层级。

admin-only 应用只有一个共享凭据且需要 cookie refresh session 时，使用 `auth/http`。
