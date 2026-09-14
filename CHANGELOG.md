# Changelog

## v0.3.1 - 2026-09-14

### Breaking

- `routes.Response` has a new optional `Headers` field. Callers using positional composite literals must switch to keyed fields. Existing keyed literals and default behavior are unchanged.
- pgx `Rows` now requires `TypeMap() *pgtype.Map`; custom implementations and mocks must add it.
- pgx connection-string parsing now follows libpq more closely: URI query `+` is literal, repeated parameters use the last value, and keyword/value backslashes require escaping. On Unix, default credential and TLS file locations now use `$HOME`.
- pgx date/time codecs reject invalid or out-of-range values. Text `timestamptz` decoding now uses `time.Local` or the configured `ScanLocation`, which can change serialized offsets without changing the instant. Set `ScanLocation` to `time.UTC` for a fixed zone. Numeric infinities now serialize as JSON strings `"Infinity"` and `"-Infinity"` instead of zero. See the [pgx v5.11.0 migration details](https://github.com/jackc/pgx/releases/tag/v5.11.0).

### Added

- Added `routes.MountWithOptions` with an application-owned unauthorized handler. Custom authentication middleware continues to use `routes.WithAuthenticated` without adopting Kit token managers, JWT claims, or principals. Existing mount functions retain the default JSON 401 guard.
- Added `routes.Response.Headers` and `routes.Header` for OpenAPI response header contracts, with case-insensitive duplicate detection and named schema support.

### Changed

- Updated Goose from v3.27.3 to v3.28.0.
- Updated pgx from v5.10.0 to v5.11.0, including Go 1.27 `database/sql` support for directly scanning PostgreSQL arrays and ranges.

### Fixed

- GORM connection initialization now uses the caller's context for its connection check, avoids the automatic context-free ping, and closes the SQL pool on verification failure.

### Security

- pgx now rejects NUL bytes in startup parameters and improves password redaction in connection-string errors. Redaction of malformed connection strings remains best effort.

## v0.3.0 - 2026-09-13

### Breaking

- Minimum Go version is now Go 1.27.1.
- `Blueprint.Get`, `Post`, `Put`, `Patch`, `Delete`, and `Handle` now accept handler functions directly through Go 1.27 generic methods constrained by `routes.HandlerFunc`. Replace `routes.Func(fn)` with `fn`; arbitrary `http.Handler` values must be added through a lower-level `routes.Route`. The `routes.Func` and `routes.Handler` adapters were removed.
- HTTP JSON handling now uses `encoding/json/v2`. Struct field matching is case-sensitive; ordinary nil slices and maps encode as `[]` and `{}`; `response.WriteJSON` does not append a newline or escape HTML characters by default; and map key order is unspecified.
- JSON v2 also changes application-defined payloads: `omitempty` no longer omits numeric zero or `false` (use `omitzero` for those fields), byte arrays encode as base64 strings, and custom marshal methods are called consistently, including on map keys. `time.Duration` has no default JSON representation; use an explicit wire type or custom marshaler. Review request and response types against the [JSON v2 contract](https://pkg.go.dev/encoding/json/v2).
- JSON decoder error details and wrapped standard-library error types now follow JSON v2. Match `decoder.ErrInvalidJSON` and `decoder.ErrBodyTooLarge` with `errors.Is` instead of matching error text or assuming JSON v1 error types.
- `routes.Route` now includes operation, parameter, body, response, and security metadata. Callers using positional composite literals must switch to keyed fields.
- Replaced `routes.ExportOpenAPI`, `routes.OpenAPIOptions`, and `Blueprint.ExportOpenAPI` with `tools/openapi.Generate`. The output moves from OpenAPI 3.0 to 3.1.1; generation requires a title, version, explicit unique operation IDs, and at least one response per operation. Pass routes with their final mount prefixes, using `Blueprint.RoutesAt` where appropriate, and update downstream generators to support OpenAPI 3.1.

### Added

- Added explicit route contracts for operation IDs, parameters, request bodies, status-specific responses, JSON schemas, and security schemes.
- Added optional `tools/openapi.Generate` output for validated OpenAPI 3.1.1 JSON, including chi path-parameter normalization and deterministic components. Route contracts describe the API; mounting does not perform runtime schema validation.
- Added `Blueprint.RoutesAt` so mounting and OpenAPI generation can consume the same final prefixed route set.
- Added complete generated contracts for the built-in `auth/http` and `auth/rbac/http` routes.

### Changed

- Chi route regular expressions are normalized out of OpenAPI paths and are not inferred as JSON Schema `pattern` constraints. Parameter constraints must be declared explicitly in route contract schemas.
- `routes.WithPrefix` now normalizes route paths and adds dynamic prefix parameters as required string path metadata unless they are declared explicitly.

### Security

- JSON request decoding now rejects duplicate object names and invalid UTF-8 by default.
- `routes.Mount` now rejects unsupported non-empty authentication requirements instead of mounting them without an authentication guard.

### Known issues

- Go 1.27.1 HTTP/1 clients can deadlock when reading and closing the same response body concurrently during automatic draining ([Go #81404](https://github.com/golang/go/issues/81404)). EiluneKit does not itself issue HTTP client requests; applications using this pattern must account for the upstream defect.
- Go 1.27.1 JSON decoding can return an overflow error after storing infinity in the destination ([Go #81062](https://github.com/golang/go/issues/81062)). This also affects the public JSON body decoder. Discard decoded results after an error; built-in auth handlers return immediately on decoding errors.
- Go 1.27.1 JSON v1 unexpectedly invokes `MarshalText` for string-kind map keys ([Go #81355](https://github.com/golang/go/issues/81355)). This can affect application or dependency code using custom key types. JSON v2 intentionally invokes these methods.

## v0.2.9 - 2026-08-17

### Breaking

- Minimum Go version is now Go 1.25.13 to pick up standard-library fixes for GO-2026-5856, GO-2026-5972, GO-2026-6088, and GO-2026-6090.
- Updated go-redis to v9.22.0. Redis clients that leave `ReadTimeout` or `WriteTimeout` unset now use 5 seconds instead of 3 seconds; the default retry backoff changed from 8ms–512ms to 10ms–1s, and TCP keep-alive now uses a 30-second idle time, a 5-second interval, and 3 probes.
- `(*redis.Client).WaitAOF` now returns `*redis.IntSliceCmd` instead of `*redis.IntCmd`; callers that name or assume the old result type must update.

### Added

- Documented opt-in go-redis Automatic Pipelining, including its context, retry, and lifecycle constraints.

### Changed

- Redis-backed session listing now reads session hashes in one pipeline instead of issuing one network round trip per session.

### Fixed

- GORM PostgreSQL translated errors now preserve the original PostgreSQL error in the error chain.

### Security

- Updated Goose and its transitive dependencies to incorporate upstream security fixes.

## v0.2.8 - 2026-08-16

### Added

- Added `migration.RunTo` for upgrade tests and maintenance workflows that need to migrate through a historical version ceiling without exposing Goose's provider.

## v0.2.7 - 2026-08-16

### Added

- Added `postgres/migration` for explicit Goose-backed PostgreSQL migrations, session advisory locking, application-owned SQL and Go migration sources, and startup checks for pending or newer schemas.

### Fixed

- Fixed session deletion for auth routes mounted below dynamic path parameters.

## v0.2.6 - 2026-06-19

### Breaking

- Login lockout now fails fast instead of silently bypassing tracking. Empty keys return `auth.ErrLockoutKeyRequired` / `rbac.ErrLockoutKeyRequired`; nil `*auth.MemoryLockout` receivers return `auth.ErrLockoutMissing`.

### Added

- Added reusable auth login lockout primitives and optional `auth/http` failed-login lockout.

### Security

- Bounded in-memory login lockout key storage and stopped RBAC HTTP login lockout keys from retaining raw usernames.

## v0.2.5 - 2026-06-19

### Added

- Added shared `auth.Principal` context helpers for authenticated users and API tokens.
- Added `auth/rbac` for multi-user auth with user status validation, role policy hooks, default in-memory login lockout, audit hooks, and opaque API token contracts.
- Added `auth/rbac/http` JSON bearer routes for login, refresh, logout, current principal, role middleware, and scope middleware.

### Changed

- Retracted `v0.2.4`; use `v0.2.5`.

### Fixed

- Mapped JWT unauthorized errors from auth HTTP middleware to `401 Unauthorized` instead of a generic auth failure.

## v0.2.3 - 2026-06-17

### Breaking

- `routes.Func` now uses a typed handler signature set for functions with optional path parameters. Existing direct calls with `http.HandlerFunc` values continue to work, but callers that pass untyped `nil` or use `routes.Func` itself as a function value must update.

### Added

- Added `routes.Func` support for passing dynamic path values as trailing `string` handler arguments, including mounted and included route prefixes.

## v0.2.2 - 2026-06-16

### Breaking

- Minimum Go version is now Go 1.25.11 to pick up standard-library fixes for GO-2026-5037 and GO-2026-5039.

## v0.2.1 - 2026-06-12

### Breaking

- `auth.Tokens` now includes `UserID`. Callers using positional composite literals must add the new field, and code that serializes, reflects on, or otherwise depends on the exported struct shape will observe the additional user ID.
- `authhttp.Options` now includes `Events` and `Logger` for login lifecycle hooks. Callers using positional composite literals must add the new fields; keyed literals such as `authhttp.Options{LoginAuthenticator: ...}` continue to work.

### Added

- Added an auth HTTP login lifecycle hook for audit and other post-issue checks.
- Added optional auth HTTP logging for lifecycle hook failures.
- Added optional strict unknown-field rejection for JSON request body decoding.

## v0.2.0 - 2026-05-22

### Breaking

- Redis-backed auth session storage moved from `auth/store` to `auth/store/redissession`; callers now construct it with `redissession.New` and `redissession.Options`.
- Minimum Go version is now Go 1.25.10.

### Changed

- Root README auth examples now document the single-process memory store limits, static password username behavior, and required login `persistence` field.

## v0.1.9 - 2026-05-19

### Breaking

- Redis-backed auth sessions now use `sessions:` keys instead of `session:` keys. Existing refresh sessions stored with the old layout are not read by the new flow, so users may need to sign in again. Old keys expire by their existing TTL.

### Added

- Added auth session listing through `GET /auth/sessions`.
- Added stored session cleanup support for memory and Redis auth stores.
- Added OpenAPI 3.0 export for route metadata.
- Added CI checks for `go vet`, Staticcheck, race tests, and govulncheck.

### Changed

- Updated the documented Go requirement to Go 1.25.
