# Changelog

## v0.1.9 - 2026-05-19

### Breaking

- Redis-backed auth sessions now use `sessions:` keys instead of `session:` keys. Existing refresh sessions stored with the old layout are not read by the new flow, so users may need to sign in again. Old keys expire by their existing TTL.
- Redis-backed auth session storage moved from `auth/store` to `auth/store/redissession`; callers now construct it with `redissession.New` and `redissession.Options`.
- Minimum Go version is now Go 1.25.10.

### Added

- Added auth session listing through `GET /auth/sessions`.
- Added stored session cleanup support for memory and Redis auth stores.
- Added OpenAPI 3.0 export for route metadata.
- Added CI checks for `go vet`, Staticcheck, race tests, and govulncheck.
