# auth/http/basic

`auth/http/basic` 将 `auth.Service` 适配为 `chi` 路由、Bearer 中间件、cookie 和 JSON 响应。导入路径：`github.com/Ithildur/EiluneKit/auth/http/basic`；包名：`basic`。

该包用于 admin-only 或共享密钥应用。它不是 RFC 7617 HTTP Basic authentication；登录使用 JSON 凭据和 refresh cookie。

## 快速开始

```go
manager, err := authjwt.New(signingKey, authstore.NewMemoryStore())
if err != nil {
	return err
}

authHandler, err := authbasic.NewHandler(manager, authbasic.Options{
	LoginAuthenticator: authbasic.LoginAuthenticatorFunc(func(ctx context.Context, username, password string) (string, bool, error) {
		user, err := repo.FindByUsername(ctx, username)
		if err != nil {
			return "", false, err
		}
		if user == nil {
			return "", false, nil
		}

		computedHash := hashPassword(password, user.Salt)
		if !authbasic.VerifyCredential(user.PasswordHash, computedHash) {
			return "", false, nil
		}
		return user.ID, true, nil
	}),
})
if err != nil {
	return err
}

if err := authHandler.Register(r); err != nil {
	return err
}
```

`POST /auth/login` 接收 `username`、`password` 和必填的 `persistence` 字段，取值为 `persistent` 或 `session`。`LoginAuthenticator` 负责校验；handler 返回 access token，并按对应策略写入 refresh/CSRF cookie。

## 固定口令

固定共享口令：

```go
staticAuth, err := authbasic.NewStaticPassword("dashboard-admin", adminPassword)
if err != nil {
	return err
}

authHandler, err := authbasic.NewHandler(manager, authbasic.Options{
	LoginAuthenticator: staticAuth,
})
if err != nil {
	return err
}
```

```go
if err := authbasic.ValidateStaticPassword(adminPassword); err != nil {
	return err
}
```

这是单管理员后台或内部控制面的预期用法。请求中的 `username` 可以被忽略；密码匹配时，配置的 user ID 会成为 token subject。

## Bearer 中间件

```go
bearer, err := authbasic.RequireBearer(manager)
if err != nil {
	return err
}
r.Use(bearer)
```

`RequireBearer` 接受任意 `AccessTokenValidator`。它会在 HTTP 层解析 `Authorization: Bearer` 请求头，而 `jwt.Manager` 只负责校验原始 access token。

公开路由需要“有 token 就挂 claims，没 token 就匿名继续”时，使用 `OptionalBearer`。它仍会拒绝格式错误或无效 token。

```go
optionalBearer, err := authbasic.OptionalBearer(manager)
if err != nil {
	return err
}
route.Get("/feed", "List feed", routes.Func(feed), routes.Auth(routes.AuthOptional), routes.Use(optionalBearer))
```

## API Key 中间件

```go
nodeKey, err := authbasic.RequireAPIKey(authbasic.APIKeyValidatorFunc(func(ctx context.Context, key string) (bool, error) {
	return subtle.ConstantTimeCompare([]byte(key), []byte(nodeSecret)) == 1, nil
}), "X-Node-Secret")
if err != nil {
	return err
}
route.Get("/node/metrics", "Node metrics", routes.Func(metrics), routes.Auth(routes.AuthRequired), routes.Use(nodeKey))
```

header 名为空时默认使用 `X-API-Key`。

## 路由

默认基础路径：`/auth`。

| 路由 | 认证 | 中间件 |
|---|---|---|
| `POST /auth/login` | `public` | 登录限流、请求体限制、JSON body |
| `POST /auth/refresh` | `required` | refresh-cookie + CSRF |
| `POST /auth/logout` | `required` | refresh-cookie + CSRF |
| `GET /auth/sessions` | `required` | `RequireBearer` |
| `DELETE /auth/sessions/current` | `required` | `RequireBearer` |
| `DELETE /auth/sessions` | `required` | `RequireBearer` |
| `DELETE /auth/sessions/{sid}` | `required` | `RequireBearer` |

`Handler.Routes()` 会返回同一组 `http/routes.Route`。返回的路由已经包含各自的认证中间件。
`GET /auth/sessions` 要求 manager 实现 `auth.SessionLister`；`auth/jwt.Manager` 在 store 支持 session listing 时支持该接口。
`DELETE /auth/sessions` 会吊销当前用户的 session；manager 支持清理时也会清理已保存的 session 记录。

```go
routeList := authHandler.Routes()
err := routes.Mount(r, "", routeList)
```

## 选项

- `LoginAuthenticator`：必需的凭据校验入口
- `BasePath`：相对于当前 router 挂载点的认证路由前缀；默认 `/auth`
- `RefreshCookiePath`：浏览器可见的 refresh cookie 路径；默认等于 `BasePath`
- `CSRFCookiePath`：CSRF cookie 路径；默认 `/`
- `RefreshCookieName`、`CSRFCookieName`、`CSRFHeaderName`：cookie 与 header 名称
- `CookieSameSite`：可选的认证 cookie `SameSite` 覆盖项；零值保持基于 TLS / 代理推导的自动行为
- `TrustedProxies`：登录限流和安全 cookie 协议判断使用的转发代理信任边界
- `MaxBodyBytes`：认证端点请求体大小限制
- `RateLimit`：登录限流配置
- `Events`：认证生命周期 hook。`Events.Login` 在凭据通过且 token 已签发后、cookie 和响应体写出前执行；返回错误会尝试吊销新的 refresh session 并让登录请求失败。hook 可能并发调用。
- `Logger`：可选 `*slog.Logger`，用于记录认证生命周期 hook 失败和补偿吊销失败。

只有设置 `TrustedProxies` 才信任转发头。默认限流 key 使用 `RemoteAddr`。

默认情况下，cookie 的 `Secure` 和 `SameSite` 会从请求推导：TLS 总是启用 secure cookie；只有可信代理发来的 `X-Forwarded-Proto: https` 才参与判断。部署策略需要固定模式时设置 `CookieSameSite`，例如同站应用使用 `http.SameSiteLaxMode`，跨站 SPA 使用 `http.SameSiteNoneMode`。

## 契约

- `NewHandler` 需要 `TokenManager` 和 `Options.LoginAuthenticator`
- `NewHandler` 接收一个 `Options` 结构体；除 `LoginAuthenticator` 外，其他字段为零值时都会回退到默认配置
- `NewStaticPassword` 需要非空 user ID 和 password
- `VerifyCredential` 使用精确字节比较，适合比较预先 hash 后或应用自行派生的凭据
- 应用需要多用户、角色或 scope 时，使用 `auth/rbac` 和 `auth/http/rbac`
