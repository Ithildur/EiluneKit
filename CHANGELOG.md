# Changelog

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
