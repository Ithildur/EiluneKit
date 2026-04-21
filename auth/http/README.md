# auth/http

`auth/http` adapts `auth.Service` to `chi` routes, bearer middleware, cookies, and JSON responses. Import path: `github.com/Ithildur/EiluneKit/auth/http`; package name: `authhttp`.

## Quick Start

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

`POST /auth/login` accepts `username`, `password`, and a required `persistence` field with `persistent` or `session`. `LoginAuthenticator` verifies the credentials; the handler returns an access token and sets refresh/CSRF cookies with matching persistence.

## Static Credential

For a fixed shared secret:

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

## Bearer Middleware

```go
bearer, err := authhttp.RequireBearer(manager)
if err != nil {
	return err
}
r.Use(bearer)
```

`RequireBearer` accepts any `AccessTokenValidator`. It parses the HTTP `Authorization: Bearer` header in the HTTP layer, while `jwt.Manager` only validates the raw access token.

## Routes

Default base path: `/auth`.

| Route | Auth |
|---|---|
| `POST /auth/login` | `routes.AuthNone` |
| `POST /auth/refresh` | `routes.AuthRefreshCookie` |
| `POST /auth/logout` | `routes.AuthRefreshCookie` |
| `DELETE /auth/sessions/current` | `routes.AuthBearerRequired` |
| `DELETE /auth/sessions` | `routes.AuthBearerRequired` |
| `DELETE /auth/sessions/{sid}` | `routes.AuthBearerRequired` |

`Handler.Routes()` returns the same route set as declarative `http/routes.Route` values. When mounting them manually, first build the resolver with `authHandler.AuthResolver()`, then pass `routes.WithAuth(resolver)`. Omitting that resolver is a mount-time error for protected routes.

```go
resolver, err := authHandler.AuthResolver()
if err != nil {
	return err
}
err = routes.Mount(r, "", authHandler.Routes(), routes.WithAuth(resolver))
```

## Options

- `LoginAuthenticator`: required credential verification entrypoint
- `BasePath`: auth route prefix relative to the current router mount; default `/auth`
- `RefreshCookiePath`: browser-visible refresh-cookie path; default `BasePath`
- `CSRFCookiePath`: CSRF cookie path; default `/`
- `RefreshCookieName`, `CSRFCookieName`, `CSRFHeaderName`: cookie and header names
- `TrustedProxies`: forwarded-header trust boundary for rate limiting and secure-cookie detection
- `MaxBodyBytes`: request body size limit for auth endpoints
- `RateLimit`: login rate-limit settings

Forwarded headers are trusted only when `TrustedProxies` is set. The default rate-limit key uses `RemoteAddr`.

## Contracts

- `NewHandler` requires both a `TokenManager` and `Options.LoginAuthenticator`.
- `NewHandler` takes one `Options` struct; fields other than `LoginAuthenticator` fall back to defaults when left zero-valued.
- `NewStaticPasswordAuthenticator` requires a non-empty user ID and password.
- `AuthResolver` may fail when the handler is nil or the bearer middleware dependencies are missing.
- `VerifyCredential` performs exact byte comparison and is suitable for pre-hashed or application-derived credentials.
