# CAPTCHA

[简体中文](README_CN.md)

`captcha` verifies tokens with Cap Standalone, Cloudflare Turnstile and Google reCAPTCHA v2/v3. `captcha/http` provides optional HTTP middleware. Applications supply constructor arguments; neither package reads configuration files or environment variables, manages credentials, nor persists tokens.

## Clients

```go
import "github.com/Ithildur/EiluneKit/captcha"

capClient, err := captcha.NewCap(
    "https://cap.example.com/site-key/siteverify",
    secret,
    captcha.Options{},
)
```

Cap must already be deployed. Use the site key's secret, not the dashboard admin key. The endpoint is the full verification URL; HTTP is supported for private service connections, and HTTPS should be used across untrusted networks.

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

Check each constructor error before using its client. Hostname and action are exact matches when supplied. reCAPTCHA v3 requires a non-empty action; its minimum score defaults to 0.5. An explicit threshold can range from 0 to 1, and the response must contain a valid score even with a zero threshold. These clients use `siteverify`, not the Google Enterprise Assessment API.

Use a separate client for each action or policy. Constructors validate local arguments without contacting providers. A constructor succeeding does not establish that its secret is accepted by the provider.

`Options.HTTPClient` optionally supplies an HTTP client; `Options.Timeout` defaults to five seconds and bounds the whole verification. The client is copied, its transport remains shared, and redirects are disabled. The application owns the transport's lifecycle. Clients support concurrent calls and do not cache verification results or automatically retry requests.

## Built-in login

Both `auth/http.Options` and `auth/rbac/http.Options` accept `Captcha captchahttp.Options`. The zero value disables verification. Set `Enabled: true` and supply `Verify: client.Verify` to protect `POST {BasePath}/login`.

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

Disabled protection never calls `Verify`. Enabled protection requires a non-nil verification function; otherwise `NewHandler` returns an error. Runtime failures never disable protection. Custom verification functions must be concurrency-safe, return nil only on success, and use the `captcha` error sentinels when applicable.

The login chain installs rate limiting, the existing body limit and Content-Type checks before CAPTCHA. The body limit takes effect when the handler reads the body. CAPTCHA runs before credential authentication; its failures do not create sessions, issue tokens, set login cookies or count as password failures. Refresh, logout and session management remain independent of CAPTCHA.

Enabled login routes declare the required `X-Captcha-Token` header and the 403 response in generated OpenAPI. Disabled routes retain their existing contract. CAPTCHA remains an additional check on a public login route, not a substitute for authentication or rate limiting.

## Custom endpoints

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

The middleware also wraps an ordinary `http.Handler`. `captchahttp.New` returns nil when disabled; only attach a non-nil middleware. It reads `X-Captcha-Token` and does not consume or rewrite the body. Applications own custom endpoint metadata, including the required header and error responses.

For tokens in JSON or form bodies, decode the request once in the handler and call `client.Verify(r.Context(), token)` before business side effects. A nil return means the token passed all configured checks. Do not verify the same token again in another layer.

## Browser integration

Use the chosen provider's widget and public site key in the browser. Keep its secret on the server. Submit the returned token with the business request:

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

For Cap, obtain the token from the widget's `solve` event (`event.detail.token`). For Turnstile or reCAPTCHA v2, use the widget callback token; for reCAPTCHA v3, use the result of `grecaptcha.execute` with the action expected by the server. Obtain a fresh token for each submission attempt, including after a failed login. Cross-origin applications must allow `X-Captcha-Token` in their CORS policy.

## Errors

| Client error (`errors.Is`) | HTTP status | Response code |
|---|---|---|
| `captcha.ErrRequired` | 400 | `captcha_required` |
| `captcha.ErrRejected` | 403 | `captcha_rejected` |
| `captcha.ErrMisconfigured` | 500 | `captcha_misconfigured` |
| `captcha.ErrUnavailable` or an unknown custom error | 503 | `captcha_unavailable` |

Invalid/expired/spent tokens and failed policies are rejected. Explicit provider credential errors are misconfiguration; network failures and unrecognized or malformed responses are unavailable. The client preserves request-context cancellation for `errors.Is`. Error text omits secrets, tokens, raw provider responses and transport error text. HTTP responses use the existing `response.ErrorResponse`; applications can wrap `Verify` to record client errors with their own logger.

Protocol references: [Cap](https://trycap.dev/guide/), [Turnstile](https://developers.cloudflare.com/turnstile/get-started/server-side-validation/), [reCAPTCHA verification](https://developers.google.com/recaptcha/docs/verify), [reCAPTCHA v3](https://developers.google.com/recaptcha/docs/v3).
