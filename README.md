# EiluneKit

Chinese: `README_CN.md`.

EiluneKit is a small Go toolkit for auth, HTTP services, Postgres, Redis, logging, and narrow runtime helpers. Import the packages you need; there is no central framework.

## Installation

```bash
go get github.com/Ithildur/EiluneKit@latest
```

Requires Go 1.26.

## Design

- `auth` owns transport-neutral login, refresh, logout, and session revocation. `auth/http` adapts it to `chi`, cookies, and JSON.
- `http/routes` keeps route metadata next to handlers. `Route` is the data model; `Blueprint` is the recommended builder.
- `http/static` mounts static files and SPA handlers from project-relative paths such as `dist` or `web/dist`.

## Usage

Start with the package docs:

- `http/routes/README.md`: route declarations, `Blueprint`, and lower-level `Route`/`Mount`
- `auth/http/README.md`: auth endpoints and bearer middleware for `chi`
- `postgres/README.md`: GORM and pgx connection helpers

## Package Layout

- `auth`: transport-neutral auth service, credential interfaces, and static password helpers
- `auth/http`: auth handlers, bearer middleware, login rate limiting, and session revocation endpoints
- `auth/jwt`: access and refresh JWT issuance backed by `auth/store`
- `auth/session`: cookie and CSRF helpers
- `auth/store`: session and token state stores for memory and Redis
- `http/decoder`: JSON request decoding helpers
- `http/middleware`: JSON-only guards, access logging, rate limits, and 404/405 helpers
- `http/response`: JSON response helpers
- `http/routes`: declarative route definitions and export helpers
- `http/static`: static file and SPA mounting helpers
- `postgres/dbtypes`: thin database type aliases
- `postgres/gorm`: Postgres DSN and `*gorm.DB` helpers
- `postgres/pgx`: Postgres DSN and `*pgxpool.Pool` helpers
- `redis`: Redis client helpers
- `logging`: slog-based logging helpers
- `appdir`: application home directory discovery
- `contextutil`: context and timeout helpers
- `clientip`: client IP extraction helpers

## Documentation

- `auth/http/README.md`
- `auth/http/README_CN.md`
- `http/routes/README.md`
- `http/routes/README_CN.md`
- `postgres/README.md`
- `postgres/README_CN.md`
- `SECURITY.md`
- `SECURITY_CN.md`

## Development

Run tests from the repository root:

```bash
go test ./...
```

## License

MIT. See `LICENSE`.
