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
