# EiluneKit

English: `README.md`.

EiluneKit 是一组小型 Go 工具包，覆盖认证、HTTP 服务、Postgres、Redis、日志和少量运行时辅助。按需导入包；这里没有中心化框架。

## 安装

```bash
go get github.com/Ithildur/EiluneKit@latest
```

需要 Go 1.26。

## 设计

- `auth` 负责与传输层无关的登录、刷新、登出和 session 吊销。`auth/http` 将它适配为 `chi`、cookie 和 JSON。
- `http/routes` 让路由元数据贴着 handler。`Route` 是数据模型；推荐用 `Blueprint` 构建。
- `http/static` 从项目内相对路径（例如 `dist`、`web/dist`）挂载静态文件和 SPA。

## 使用

从包文档开始：

- `http/routes/README_CN.md`：路由声明、`Blueprint` 和更底层的 `Route`/`Mount`
- `auth/http/README_CN.md`：面向 `chi` 的认证端点和 Bearer 中间件
- `postgres/README_CN.md`：GORM 与 pgx 连接辅助

## 包布局

- `auth`：与传输层无关的认证 service、凭据接口和固定密码辅助
- `auth/http`：认证 handler、Bearer 中间件、登录限流和会话吊销端点
- `auth/jwt`：由 `auth/store` 支撑的 access / refresh JWT 签发与校验
- `auth/session`：cookie 与 CSRF 辅助
- `auth/store`：memory 与 Redis 的 session / token 状态存储
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

- `auth/http/README.md`
- `auth/http/README_CN.md`
- `http/routes/README.md`
- `http/routes/README_CN.md`
- `postgres/README.md`
- `postgres/README_CN.md`
- `SECURITY.md`
- `SECURITY_CN.md`

## 开发

在仓库根目录运行测试：

```bash
go test ./...
```

## 许可证

MIT。见 `LICENSE`。
