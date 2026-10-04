# auth/rbac/http

`auth/rbac/http` adapts `auth/rbac.Service` to JSON bearer routes.

Use it for applications that need multiple users, role checks, scope checks, or opaque API tokens. `auth/rbac` owns the authentication flow; the application owns user storage, password hashing, role assignment, and token persistence.

Default HTTP paths:

| Route | Purpose |
|---|---|
| `POST /auth/login` | returns `access_token`, `refresh_token`, and `user` |
| `POST /auth/refresh` | rotates `refresh_token` from JSON body |
| `POST /auth/logout` | revokes `refresh_token` from JSON body |
| `GET /auth/me` | returns the current principal |

`POST /auth/login` accepts `username`, `password`, and optional `persistence`. Missing `persistence` defaults to persistent tokens; `session` returns session-only token metadata.

Login is rate-limited to five requests per minute per client IP prefix (IPv4 /24, IPv6 /40) by default. `Options.RateLimit` uses the same options as `auth/http`; set `Disabled` when an outer limiter owns this policy. Trusted proxies and client IP headers inherit from the handler unless overridden in the rate-limit options. Refresh and other endpoints do not use this login limiter.

Credential, lockout, and capacity rejections all return the same `401 unauthorized` response with `invalid credentials`, without distinct response headers. Rate limiting independently returns `429 rate_limited`. Backend failures retain their normal error responses. Timing is not equalized, and `RejectNewKeys` can reject valid credentials when full. See [lockout capacity policies](../README.md#service-setup).

Mount and generate the contract from the same route values:

```go
routeList := authHandler.Routes()
err := routes.Mount(r, "", routeList)

spec, err := openapi.Generate(routeList, openapi.Options{
	Title:   "Application RBAC API",
	Version: "1.0.0",
})
```

`Options.BasePath` is a `*string` route prefix: `nil` defaults to `"/auth"`, `new("")` selects the root, and `new("/api/auth")` selects `/api/auth`. Non-empty prefixes require a single leading slash; trailing slashes and surrounding whitespace are rejected.

## Recommended Stack

Login lockout uses the client IP and a hash of the submitted username. Configure `Options.TrustedProxies` to accept forwarded client IPs and `Options.ClientIPHeaders` to select their priority. Nil uses `X-Forwarded-For`, `X-Real-IP`, `Forwarded`, `True-Client-IP`, then `CF-Connecting-IP`; `[]string{}` disables forwarded IP headers. A non-empty list replaces the defaults. Only list headers your trusted proxy sanitizes or sets. The handler copies these configuration slices at construction.

- `auth/rbac.Service` with application `UserStore` and `PasswordVerifier`
- `auth/jwt.Manager` with `auth/store/redissession` for multi-instance session state
- `auth/rbac.Lockout`; `auth/rbac.Service` defaults to in-memory lockout, so pass Redis or SQL-backed lockout for multi-instance deployments
- `auth/rbac.APITokenStore` when API tokens should authenticate through the same Bearer middleware
- `Options.RolePolicy` for role hierarchy, or the default exact-role policy for simple projects

Middleware helpers are available from `Handler.Middleware()` or `NewMiddleware`:

```go
authz := authHandler.Middleware()
r.Use(authz.RequireAuth())
r.With(authz.RequireRole("admin")).Get("/admin", adminHandler)
r.With(authz.RequireScope("vm:read")).Get("/vms", listVMs)
```

Configure role hierarchy with `Options.RolePolicy` or `NewMiddleware(service, policy)`.

Use `auth/http` instead for admin-only applications that only need one shared credential and cookie-backed refresh sessions.
