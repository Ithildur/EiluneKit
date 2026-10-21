# CAPTCHA

[English](README.md)

`captcha` 对接 Cap Standalone、Cloudflare Turnstile 和 Google reCAPTCHA v2/v3，`captcha/http` 提供可选 HTTP 中间件。应用传入构造参数；两个包均不读取配置文件或环境变量、不管理凭据、不持久化 token。

## 客户端

```go
import "github.com/Ithildur/EiluneKit/captcha"

capClient, err := captcha.NewCap(
    "https://cap.example.com/site-key/siteverify",
    secret,
    captcha.Options{},
)
```

Cap 需要单独部署。使用站点密钥对应的 secret，不是管理后台的 admin key。端点为完整验证 URL；支持私有服务连接使用 HTTP，跨越不可信网络时应使用 HTTPS。

```go
turnstile, err := captcha.NewTurnstile(secret, captcha.TurnstileOptions{
    Hostname: "example.com",
    Action:   "login",
})

recaptchaV2, err := captcha.NewRecaptchaV2(secret, captcha.RecaptchaV2Options{
    Hostname: "example.com",
})

recaptchaV3, err := captcha.NewRecaptchaV3(secret, captcha.RecaptchaV3Options{
    Hostname: "example.com",
    Action:   "login",
    MinScore: new(0.5),
})
```

使用客户端前检查各构造函数的错误。传入 hostname 和 action 时执行精确匹配。reCAPTCHA v3 要求 action 非空；最低评分默认为 0.5，显式阈值可取 0 到 1，即使阈值为零也要求响应包含有效评分。这些客户端使用 `siteverify`，不对接 Google Enterprise Assessment API。

为不同 action 或策略分别构造客户端。构造函数仅检查本地参数，不联系服务商；构造成功不代表服务商已确认 secret 有效。

`Options.HTTPClient` 可注入 HTTP 客户端；`Options.Timeout` 默认为五秒，限制整个验证耗时。HTTP 客户端会被复制，transport 仍然共享，并禁止重定向。transport 生命周期由应用负责。客户端支持并发调用，不缓存验证结果、不自动重试。

## 内置登录

`auth/http.Options` 和 `auth/rbac/http.Options` 均接受 `Captcha captchahttp.Options`。零值关闭验证；设置 `Enabled: true` 并传入 `Verify: client.Verify` 后，保护 `POST {BasePath}/login`。

```go
import (
    authhttp "github.com/Ithildur/EiluneKit/auth/http"
    "github.com/Ithildur/EiluneKit/captcha"
    captchahttp "github.com/Ithildur/EiluneKit/captcha/http"
)

func newAuthHandler(manager authhttp.TokenManager, login authhttp.LoginAuthenticator,
    enabled bool, secret string,
) (*authhttp.Handler, error) {
    protection := captchahttp.Options{Enabled: enabled}
    if enabled {
        client, err := captcha.NewTurnstile(secret, captcha.TurnstileOptions{
            Hostname: "example.com",
            Action:   "login",
        })
        if err != nil {
            return nil, err
        }
        protection.Verify = client.Verify
    }
    return authhttp.NewHandler(manager, authhttp.Options{
        LoginAuthenticator: login,
        Captcha:            protection,
    })
}
```

关闭时不调用 `Verify`；启用时验证函数不能为 nil，否则 `NewHandler` 返回错误。运行时失败不会关闭保护。自定义验证函数必须支持并发，仅在验证成功时返回 nil，并按需使用 `captcha` 包的错误哨兵。

登录链先安装限流、原有请求体限制和 Content-Type 检查，再执行 CAPTCHA。请求体限制在 handler 读取 body 时生效。CAPTCHA 位于凭据认证之前；验证失败不会创建 session、签发 token、设置登录 cookie 或计入密码失败次数。刷新、退出和会话管理不受 CAPTCHA 影响。

启用后，登录路由生成的 OpenAPI 声明必填 `X-Captcha-Token` 请求头及 403 响应；关闭时保持原契约。CAPTCHA 是公开登录入口的附加检查，不代替认证或限流。

## 自定义端点

```go
guard, err := captchahttp.New(captchahttp.Options{
    Enabled: true,
    Verify:  client.Verify,
})
if err != nil {
    return err
}
blueprint.Post("/signup", "Sign up", signup, routes.Use(guard))
```

中间件也可以包装普通 `http.Handler`。关闭时 `captchahttp.New` 返回 nil，只挂载非 nil 中间件。它读取 `X-Captcha-Token`，不消费或改写请求体。自定义端点的请求头及错误响应元数据由应用声明。

如果 token 放在 JSON 或表单中，handler 解码一次请求后，在业务副作用前调用 `client.Verify(r.Context(), token)`。返回 nil 表示 token 满足全部校验要求。不要在另一层重复验证同一个 token。

## 浏览器接入

浏览器使用对应服务商的组件及公开 site key，secret 留在服务端。把组件返回的 token 与业务请求一起提交：

```js
await fetch("/auth/login", {
  method: "POST",
  headers: {
    "Content-Type": "application/json",
    "X-Captcha-Token": token,
  },
  body: JSON.stringify({
    username,
    password,
    persistence: "session",
  }),
});
```

Cap 从组件的 `solve` 事件取得 `event.detail.token`。Turnstile 和 reCAPTCHA v2 使用组件回调传入的 token；reCAPTCHA v3 使用 `grecaptcha.execute` 的结果，action 必须与服务端预期一致。每次提交都应取得新 token，包括登录失败后的再次尝试。跨域应用需要在 CORS 策略中允许 `X-Captcha-Token`。

## 错误

| 客户端错误（`errors.Is`） | HTTP 状态 | 响应 code |
|---|---|---|
| `captcha.ErrRequired` | 400 | `captcha_required` |
| `captcha.ErrRejected` | 403 | `captcha_rejected` |
| `captcha.ErrMisconfigured` | 500 | `captcha_misconfigured` |
| `captcha.ErrUnavailable` 或未知自定义错误 | 503 | `captcha_unavailable` |

无效、过期、已消费的 token 以及策略不匹配均被拒绝。服务商明确报告的凭据错误归为配置错误；网络故障、无法识别或格式错误的响应归为上游不可用。请求 context 的取消可以通过 `errors.Is` 判断。错误文本不包含 secret、token、上游原始响应或传输错误文本。HTTP 响应复用 `response.ErrorResponse`；应用可以包装 `Verify`，使用自己的 logger 记录客户端错误。

协议参考：[Cap](https://trycap.dev/guide/)、[Turnstile](https://developers.cloudflare.com/turnstile/get-started/server-side-validation/)、[reCAPTCHA 验证](https://developers.google.com/recaptcha/docs/verify)、[reCAPTCHA v3](https://developers.google.com/recaptcha/docs/v3)。
