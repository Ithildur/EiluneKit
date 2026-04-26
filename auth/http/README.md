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

Use `OptionalBearer` when a public route may attach user claims but must still reject malformed or invalid tokens.

```go
optionalBearer, err := authhttp.OptionalBearer(manager)
if err != nil {
	return err
}
route.Get("/feed", "List feed", routes.Func(feed), routes.Auth(routes.AuthOptional), routes.Use(optionalBearer))
```

## API Key Middleware

```go
nodeKey, err := authhttp.RequireAPIKey(authhttp.APIKeyValidatorFunc(func(ctx context.Context, key string) (bool, error) {
	return subtle.ConstantTimeCompare([]byte(key), []byte(nodeSecret)) == 1, nil
}), "X-Node-Secret")
if err != nil {
	return err
}
route.Get("/node/metrics", "Node metrics", routes.Func(metrics), routes.Auth(routes.AuthRequired), routes.Use(nodeKey))
```

An empty header name defaults to `X-API-Key`.

## Routes

Default base path: `/auth`.

| Route | Auth | Middleware |
|---|---|---|
| `POST /auth/login` | `public` | login rate limit, body limit, JSON body |
| `POST /auth/refresh` | `required` | refresh-cookie + CSRF |
| `POST /auth/logout` | `required` | refresh-cookie + CSRF |
| `DELETE /auth/sessions/current` | `required` | `RequireBearer` |
| `DELETE /auth/sessions` | `required` | `RequireBearer` |
| `DELETE /auth/sessions/{sid}` | `required` | `RequireBearer` |

`Handler.Routes()` returns the same route set as declarative `http/routes.Route` values. Returned routes already contain their auth middleware.

```go
routeList := authHandler.Routes()
err := routes.Mount(r, "", routeList)
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
- `VerifyCredential` performs exact byte comparison and is suitable for pre-hashed or application-derived credentials.
