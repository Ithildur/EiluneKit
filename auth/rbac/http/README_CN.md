# auth/rbac/http

`auth/rbac/http` 将 `auth/rbac.Service` 适配为 JSON bearer 路由。

它用于需要多用户、角色检查、scope 检查或 opaque API token 的应用。该包只暴露传输层；认证流程仍由 `auth/rbac` 负责，用户存储、密码 hash、角色分配和 token 持久化仍由应用负责。

默认路由挂在 `/auth` 下：

| 路由 | 用途 |
|---|---|
| `POST /auth/login` | 返回 `access_token`、`refresh_token` 和 `user` |
| `POST /auth/refresh` | 从 JSON body 轮换 `refresh_token` |
| `POST /auth/logout` | 从 JSON body 吊销 `refresh_token` |
| `GET /auth/me` | 返回当前主体 |

`POST /auth/login` 接收 `username`、`password` 和可选的 `persistence`。缺省 `persistence` 表示持久 token；`session` 返回 session-only token 元数据。

## 推荐组合

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

角色层级是应用策略。通过 `Options.RolePolicy` 或 `NewMiddleware(service, policy)` 传入；本包不知道 `admin`、`operator`、`viewer`、`vm_user` 这些业务角色。

admin-only 应用只有一个共享凭据且需要 cookie refresh session 时，使用 `auth/http`。
