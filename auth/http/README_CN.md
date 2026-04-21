# auth/http

`auth/http` 将 `auth.Service` 适配为 `chi` 路由、Bearer 中间件、cookie 和 JSON 响应。导入路径：`github.com/Ithildur/EiluneKit/auth/http`；包名：`authhttp`。

## 快速开始

```go
manager, err := authjwt.New(signingKey, authstore.NewMemoryStore())
if err != nil {
	return err
}

authHandler, err := authhttp.NewHandler(manager, authhttp.Options{
	LoginAuthenticator: authhttp.LoginAuthenticatorFunc(func(ctx context.Context, username, password string) (string, bool, error) {
		user, err := repo.FindByUsername(ctx, username)
		if err != nil {
			return "", false, err
		}
		if user == nil {
			return "", false, nil
		}

		computedHash := hashPassword(password, user.Salt)
		if !authhttp.VerifyCredential(user.PasswordHash, computedHash) {
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
staticAuth, err := authhttp.NewStaticPasswordAuthenticator("dashboard-admin", adminPassword)
if err != nil {
	return err
}

authHandler, err := authhttp.NewHandler(manager, authhttp.Options{
	LoginAuthenticator: staticAuth,
})
if err != nil {
	return err
}
```

```go
if err := authhttp.ValidateStaticPasswordVisibleASCII(adminPassword); err != nil {
	return err
}
```

## Bearer 中间件

```go
bearer, err := authhttp.RequireBearer(manager)
if err != nil {
	return err
}
r.Use(bearer)
```

`RequireBearer` 接受任意 `AccessTokenValidator`。它会在 HTTP 层解析 `Authorization: Bearer` 请求头，而 `jwt.Manager` 只负责校验原始 access token。

## 路由

默认基础路径：`/auth`。

| 路由 | 认证 |
|---|---|
| `POST /auth/login` | `routes.AuthNone` |
| `POST /auth/refresh` | `routes.AuthRefreshCookie` |
| `POST /auth/logout` | `routes.AuthRefreshCookie` |
| `DELETE /auth/sessions/current` | `routes.AuthBearerRequired` |
| `DELETE /auth/sessions` | `routes.AuthBearerRequired` |
| `DELETE /auth/sessions/{sid}` | `routes.AuthBearerRequired` |

`Handler.Routes()` 会返回同一组 `http/routes.Route`。如果你手工挂载它们，必须先通过 `authHandler.AuthResolver()` 构建 resolver，再传入 `routes.WithAuth(resolver)`。对受保护路由来说，缺少这个 resolver 会在挂载时直接报错。

```go
resolver, err := authHandler.AuthResolver()
if err != nil {
	return err
}
err = routes.Mount(r, "", authHandler.Routes(), routes.WithAuth(resolver))
```

## 选项

- `LoginAuthenticator`：必需的凭据校验入口
- `BasePath`：相对于当前 router 挂载点的认证路由前缀；默认 `/auth`
- `RefreshCookiePath`：浏览器可见的 refresh cookie 路径；默认等于 `BasePath`
- `CSRFCookiePath`：CSRF cookie 路径；默认 `/`
- `RefreshCookieName`、`CSRFCookieName`、`CSRFHeaderName`：cookie 与 header 名称
- `TrustedProxies`：登录限流和安全 cookie 协议判断使用的转发代理信任边界
- `MaxBodyBytes`：认证端点请求体大小限制
- `RateLimit`：登录限流配置

只有设置 `TrustedProxies` 才信任转发头。默认限流 key 使用 `RemoteAddr`。

## 契约

- `NewHandler` 需要 `TokenManager` 和 `Options.LoginAuthenticator`
- `NewHandler` 接收一个 `Options` 结构体；除 `LoginAuthenticator` 外，其他字段为零值时都会回退到默认配置
- `NewStaticPasswordAuthenticator` 需要非空 user ID 和 password
- `AuthResolver` 在 handler 为 nil 或缺少 Bearer 中间件依赖时会返回错误
- `VerifyCredential` 使用精确字节比较，适合比较预先 hash 后或应用自行派生的凭据
