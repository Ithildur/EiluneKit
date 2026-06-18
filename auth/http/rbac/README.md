# auth/http/rbac

`auth/http/rbac` adapts `auth/rbac.Service` to JSON bearer routes.

Use it for applications that need multiple users, role checks, scope checks, or opaque API tokens. It exposes the transport layer only; `auth/rbac` still owns the auth flow and the application still owns user storage, password hashing, role assignment, and token persistence.

Default routes under `/auth`:

| Route | Purpose |
|---|---|
| `POST /auth/login` | returns `access_token`, `refresh_token`, and `user` |
| `POST /auth/refresh` | rotates `refresh_token` from JSON body |
| `POST /auth/logout` | revokes `refresh_token` from JSON body |
| `GET /auth/me` | returns the current principal |

`POST /auth/login` accepts `username`, `password`, and optional `persistence`. Missing `persistence` defaults to persistent tokens; `session` returns session-only token metadata.

## Recommended Stack

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

Role hierarchy is application policy. Pass `Options.RolePolicy` or `NewMiddleware(service, policy)`; this package does not know roles such as `admin`, `operator`, `viewer`, or `vm_user`.

Use `auth/http/basic` instead for admin-only applications that only need one shared credential and cookie-backed refresh sessions.
