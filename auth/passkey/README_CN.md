# 通行密钥

[English](README.md)

`passkey.Service` 负责 WebAuthn 凭据注册和身份验证，返回已验证身份；现有 auth 服务负责建立会话和应用 RBAC。核心不依赖密码，不生成私钥、不同步 Google 账户，也不分配权限。

注册要求可发现凭据和用户验证，按顺序提供 ML-DSA-44（`-48`）、ES256（`-7`）。这是算法协商，不是双重签名；每个凭据保持注册时的算法，登录根据已存公钥验证。Google Password Manager 与其他提供方共用协议，浏览器及用户选择保存位置。服务端不能强制存入 Google，也不承诺 Google 的 ML-DSA 支持。仍允许传统凭据或密码恢复的账户不能称为全面抗量子。

## 配置

应用提供凭据持久化和当前有效用户查询。用户 ID 必须不变、不含个人信息、非空、无首尾空白且永不重新分配，不能使用邮箱或用户名。WebAuthn 用户句柄由 RP ID 与账户 ID 派生为稳定 SHA-256 值；改变任一标识会使已有凭据失效。

```go
keys, err := passkey.New(passkey.Options{
    RPID:          "example.com",
    RPDisplayName: "Example",
    Origins:       []string{"https://example.com"},
    Store:         credentialStore,
    Flows:         passkey.NewMemoryFlowStore(10000),
    LookupUser:    lookupActiveUser,
})
if err != nil {
    return err
}
```

`LookupUser` 签名为 `func(context.Context, string) (passkey.User, bool, error)`。账户不存在或被禁用时返回 `false, nil`，后端故障返回错误。提供当前 `Name`、`DisplayName` 供账户选择器显示。注册、凭据管理以及登录签名验证成功后都会查询当前账户。

凭据存储和流程命名空间应限定到当前 RP。显式配置 Origins，不从不可信请求头推断。流程默认五分钟有效。内存流程存储容量有上限，仅供单进程使用，重启后失效，无需关闭后台 goroutine。满载时拒绝新流程，不驱逐有效挑战。多实例使用 Redis：

```go
flows, err := redisflow.New(redisClient, "example.com:passkey:flow:")
if err != nil {
    return err
}
// 将 flows 传入 passkey.Options.Flows。
```

要求 Redis 6.2+，客户端启用 `ContextTimeoutEnabled: true`；调用方管理客户端生命周期。操作限时五秒，故障不会回退到内存。`GETDEL` 原子消费挑战。finish 失败后必须重新 begin，包括挑战消费后的后端失败；请求超时也可能已消费挑战。

## 按需接入会话

普通 auth：

```go
handler, err := authhttp.NewHandler(manager, authhttp.Options{
    LoginAuthenticator: existingPasswordAuthenticator,
    Passkeys: keys,
})
```

`login.finish` 使用现有会话签发，写刷新及 CSRF Cookie，返回 `access_token`、`expires_at`、`csrf_token`。现有登录事件在响应前执行，失败则撤销新刷新会话。Passkey 登录事件的 `Username` 为空，使用 `UserID` 标识账户。

RBAC：

```go
// 先构造现有 rbac.Service。
keys, err := passkey.New(passkey.Options{
    RPID: "example.com", RPDisplayName: "Example",
    Origins: []string{"https://example.com"},
    Store: credentialStore, Flows: flows,
    LookupUser: rbacService.PasskeyUser,
})
if err != nil {
    return err
}
handler, err := rbachttp.NewHandler(rbacService, rbachttp.Options{Passkeys: keys})
```

RBAC 签发前重新加载有效用户，执行现有成功事件，继续返回包含刷新 Token、Principal 和会话信息的登录 JSON。角色和权限来自当前用户存储，不写入通行密钥。API token 主体不能绑定或管理用户凭据。

两种接入可独立启用。现有构造函数仍保持密码依赖要求，但 Passkey 请求不会执行密码验证。`Passkeys: nil` 不新增路由。自行管理会话的应用可直接调用 `BeginRegistration`、`FinishRegistration`、`BeginLogin` 和 `FinishLogin`，注册及管理操作必须先授权用户 ID；不能将客户端提交的任意用户 ID 当作已验证身份。

## HTTP 契约

路径遵循现有 `BasePath`，默认 `/auth`。`POST /auth/passkey` 使用 JSON 中的 `operation`：

| operation | 其他字段 | 认证要求 | 成功响应 |
|---|---|---|---|
| `register.begin` | 无 | 用户 Bearer | `flow_id`、`options.publicKey` |
| `register.finish` | `flow_id`、`name`、`credential` | 同一用户 Bearer | 凭据元数据 |
| `login.begin` | `persistence`：`session` 或 `persistent` | 公开 | `flow_id`、`options.publicKey` |
| `login.finish` | `flow_id`、`credential` | 公开 | 所选 auth 适配器的原有登录响应 |

`credential` 为 `PublicKeyCredential.toJSON()` 返回的对象。持久化偏好在 begin 绑定，finish 传入另一值不会改变它。挑战绑定操作、RP 和注册用户，只能消费一次。名称去除首尾空白后为 1–100 个 Unicode 字符。

`GET /auth/passkeys` 返回 `{"credentials": [...]}`；`PATCH /auth/passkeys/{id}` 接收 `{"name":"Phone"}`；`DELETE /auth/passkeys/{id}` 删除凭据。两个修改操作返回 204。管理接口只接受凭据所属用户的 Bearer。ID 是无填充 base64url；元数据包含 `id`、`name`、`created_at` 和可选 `last_used_at`，不含密钥。删除不撤销已有会话，需要时组合现有撤销能力。

流程入口继承请求体限制，并使用相同 `RateLimit` 配置的独立限流器；begin、finish 各计一次请求。密码锁定和 CAPTCHA 继续保护密码登录；Passkey 请求不增加或清除密码失败记录。无效断言、失效账户或已消费流程返回 401，所属凭据不存在返回 404，重复注册返回 409，后端故障保持服务端错误。混合流程路由的生成契约在摘要中说明注册所需认证，运行时强制验证用户 Bearer。

## 持久化与浏览器示例

[`examples/passkey/postgres.go`](../../examples/passkey/postgres.go) 与 [`schema.sql`](../../examples/passkey/schema.sql) 演示应用自有 PostgreSQL 存储，不是自动执行的迁移，也不要求应用采用该 schema。应用按自身模型补充用户外键和账户删除策略。

- `(rp_id, id)` 必须唯一。创建不能覆盖记录或改变归属。
- 完整保存 `webauthn.Credential`，包括公钥、标志、认证器状态和扩展；长期凭据与短期挑战分开存储。
- 使用状态更新必须同时匹配凭据与用户，只更新已有记录，保留密钥和名称，禁止 upsert。并发请求乱序完成时保留最大计数器及已经建立的用户验证状态。同步凭据可使用零计数器，kit 遵循库的计数器及备份语义，不额外强制严格递增。
- 数据库错误必须向上传播，使用状态保存成功后才能签发会话。

[`client.js`](../../examples/passkey/client.js) 使用浏览器 JSON 转换 API，无提供方或算法分支。从配置的 HTTPS origin 提供此模块（开发可用 localhost），调用 `registerPasskey(accessToken, name)` 绑定，调用 `loginWithPasskey()` 登录。按选用适配器的约定接收会话。新用户注册、账户恢复、管理 UI 和 Google 同步分别由应用或凭据提供方负责。
