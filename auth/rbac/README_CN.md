# auth/rbac

`auth/rbac` 是面向多用户应用的传输层无关认证核心，覆盖角色、scope、基于 session 的 JWT，以及 opaque API token。

它刻意不负责用户 CRUD、密码 hash 策略、注册、邀请、找回密码、MFA、OAuth/OIDC/SSO、租户成员关系、角色存储、API token 存储或审计日志存储。这些属于应用数据和产品策略。

## 推荐组合

- `UserStore`：从应用数据库按 ID 和 username 加载用户。
- `PasswordVerifier`：用应用保存的密码 hash 校验提交的密码。
- `auth/jwt.Manager`：签发并校验由 session 状态支撑的 access/refresh token。
- `auth/store/redissession`：多实例部署使用的共享 Redis session 存储。
- `auth/store.MemoryStore`：开发环境或单进程内部工具使用的进程内 session 存储。
- `Lockout`：`NewService` 默认使用适合单进程的 `NewMemoryLockout`；多实例部署应传入 Redis 或 SQL 版 `Lockout`。
- `RolePolicy`：默认精确匹配；角色有层级时传入 `RoleAllows`。
- `APITokenStore`：只持久化 token hash，不保存原始 API token。
- `Events`：写审计日志，或在认证生命周期事件后触发补偿 hook。

## Service 组合

```go
manager, err := authjwt.New(signingKey, sessionStore)
if err != nil {
	return err
}

authz, err := rbac.NewService(rbac.ServiceOptions{
	Users: users,
	Passwords: rbac.PasswordVerifierFunc(func(ctx context.Context, user rbac.User, password string) (bool, error) {
		return verifyPassword(user.ID, password)
	}),
	Tokens:    manager,
	APITokens: apiTokens,
	Lockout:   rbac.NewMemoryLockout(rbac.MemoryLockoutOptions{}),
})
if err != nil {
	return err
}
```

多实例部署时，用 Redis 或数据库支撑的实现替换默认内存 `Lockout`，让登录失败次数在进程间保持一致。多个进程都接受 API token 时，`APITokenStore` 也应使用共享存储。

## Principal

用户认证成功后返回 `auth.Principal`：

- `Subject`：user ID
- `Username`：username
- `Role`：从 `UserStore` 加载的当前角色
- `Scopes`：从 `UserStore` 加载的当前 scope
- `Kind`：`auth.PrincipalKindUser`

`AuthenticateBearer` 每次接受 access token 时都会重新加载用户，所以禁用用户、角色变更和 scope 变更不需要等 access token 过期才生效。

API token 会认证为 `auth.PrincipalKindAPIToken`，角色和 scope 来自 `APITokenStore`。

## 边界

- 登录凭据错误或用户禁用返回 `ok=false`。
- 存储或密码校验器失败返回 `error`。
- Refresh 会先校验 refresh token，重新加载当前用户，再轮换 refresh token。
- Logout 会吊销 refresh session。
- API token 只生成一次、只返回一次明文，按 hash 加载，并且只在 service 接受后标记为已使用。
- hook 可能并发调用，返回后不得继续持有 `context.Context`。

需要通过 JSON bearer HTTP 路由和角色/scope middleware 暴露同一套服务时，使用 `auth/rbac/http`。
