# auth/http

`auth/http` contains HTTP adapters for auth packages.

Use `auth/http/basic` for admin-only, single-user, or shared-secret applications that need cookie refresh sessions, CSRF protection, and optional bearer/API-key middleware. Despite the package name, it is not RFC 7617 HTTP Basic authentication.

Use `auth/http/rbac` for multi-user JSON bearer auth with login, refresh, logout, current-principal, role middleware, scope middleware, and optional opaque API tokens.
