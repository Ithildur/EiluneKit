# EiluneKit

**资源：** [English](README.md)。

EiluneKit 是一组小型 Go 工具包，覆盖认证、HTTP 服务、Postgres、Redis、日志和少量运行时辅助。按需导入包；这里没有中心化框架。

## 安装

```bash
go get github.com/Ithildur/EiluneKit@latest
```

需要 Go 1.25.11 或更高版本。

## 设计

- `auth` 提供通用 principal 辅助和与传输层无关的认证流程。`auth/http` 将默认 session auth flow 适配到 HTTP；`auth/rbac` 和 `auth/rbac/http` 处理多用户 JSON bearer 认证。
- `http/routes` 让路由元数据贴着 handler。`Route` 是数据模型；推荐用 `Blueprint` 构建。
- `http/static` 从项目内相对路径（例如 `dist`、`web/dist`）挂载静态文件和 SPA。

## 使用

不使用 Redis 或 Postgres 的单进程最小认证组合：

```go
store := authstore.NewMemoryStore()

manager, err := authjwt.New(signingKey, store)
if err != nil {
	return err
}

login, err := authhttp.NewStaticPassword("dashboard-admin", adminPassword)
if err != nil {
	return err
}

authHandler, err := authhttp.NewHandler(manager, authhttp.Options{
	LoginAuthenticator: login,
})
if err != nil {
	return err
}

if err := authHandler.Register(r); err != nil {
	return err
}
```

固定口令认证会忽略请求中的 username；密码匹配时返回配置的 user ID。`POST /auth/login` 仍必须提供 `persistence`，取值为 `session` 或 `persistent`。

该组合使用单进程内 session store；多实例之间不共享 session，进程重启后 session 会失效。

从包文档开始：

- `http/routes/README_CN.md`：路由声明、`Blueprint` 和更底层的 `Route`/`Mount`
- `auth/http/README_CN.md`：面向 `chi` 的单用户 cookie 认证端点和 Bearer 中间件
- `auth/rbac/http/README_CN.md`：多用户 JSON bearer 认证端点和角色 / scope 中间件
- `postgres/README_CN.md`：GORM 与 pgx 连接辅助
- `redis/README_CN.md`：Redis client 构造与 TLS 选项

## 包布局

- `auth`：通用 principal 辅助、与传输层无关的认证 service、凭据接口、固定密码辅助和登录锁定基础能力
- `auth/rbac`：多用户认证 service、principal 加载、角色策略、登录锁定、API token 和审计 hook 契约
- `auth/http`：默认 session auth handler、Bearer 中间件、登录限流、可选登录锁定和会话吊销端点
- `auth/rbac/http`：JSON bearer 认证 handler 和 RBAC 中间件
- `auth/jwt`：由 `auth/store` 支撑的 access / refresh JWT 签发与校验
- `auth/session`：cookie 与 CSRF 辅助
- `auth/store`：session / token 状态接口与 memory store
- `auth/store/redissession`：Redis 版认证 session store
- `http/decoder`：JSON 请求解码辅助
- `http/middleware`：RequireJSONBody、访问日志、限流和 404/405 辅助
- `http/response`：JSON 响应辅助
- `http/routes`：声明式路由定义与导出辅助
- `http/static`：静态文件与 SPA 挂载辅助
- `postgres/dbtypes`：薄数据库类型别名
- `postgres/gorm`：Postgres DSN 与 `*gorm.DB` 辅助
- `postgres/pgx`：Postgres DSN 与 `*pgxpool.Pool` 辅助
- `redis`：Redis 客户端辅助
- `logging`：基于 `slog` 的日志辅助
- `appdir`：应用 home 目录发现
- `contextutil`：context 与超时辅助
- `clientip`：客户端 IP 提取辅助

## 文档

- `auth/rbac/README.md`
- `auth/rbac/README_CN.md`
- `auth/http/README.md`
- `auth/http/README_CN.md`
- `auth/rbac/http/README.md`
- `auth/rbac/http/README_CN.md`
- `http/routes/README.md`
- `http/routes/README_CN.md`
- `postgres/README.md`
- `postgres/README_CN.md`
- `redis/README.md`
- `redis/README_CN.md`
- `SECURITY.md`
- `SECURITY_CN.md`

## 开发

在仓库根目录运行测试：

```bash
go test ./...
```

## 许可证

MIT。见 `LICENSE`。
