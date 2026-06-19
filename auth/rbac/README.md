# auth/rbac

`auth/rbac` is the transport-neutral auth core for multi-user applications that need roles, scopes, session-backed JWTs, and opaque API tokens.

It deliberately does not own user CRUD, password hashing policy, registration, invitations, password reset, MFA, OAuth/OIDC/SSO, tenant membership, role storage, API-token storage, or audit-log storage. Those are application data and product policy.

## Recommended Stack

- `UserStore`: load users by ID and username from the application database.
- `PasswordVerifier`: verify the submitted password against the application's stored password hash.
- `auth/jwt.Manager`: issue and validate access/refresh tokens backed by session state.
- `auth/store/redissession`: shared Redis session storage for multi-instance deployments.
- `auth/store.MemoryStore`: in-process session storage for development or one-process internal tools.
- `Lockout`: `NewService` defaults to `NewMemoryLockout` for one process; pass a Redis or SQL-backed `Lockout` for multi-instance deployments.
- `RolePolicy`: exact match by default; pass `RoleAllows` when roles form a hierarchy.
- `APITokenStore`: persist only token hashes, not raw API tokens.
- `Events`: write audit logs or trigger compensation hooks after auth lifecycle events.

## Service Setup

```go
manager, err := authjwt.New(signingKey, sessionStore)
if err != nil {
	return err
}

authz, err := rbac.NewService(rbac.ServiceOptions{
	Users: users,
	Passwords: rbac.PasswordVerifierFunc(func(ctx context.Context, user rbac.User, password string) (bool, error) {
		return verifyPassword(user.ID, password)
	}),
	Tokens:    manager,
	APITokens: apiTokens,
	Lockout:   rbac.NewMemoryLockout(rbac.MemoryLockoutOptions{}),
})
if err != nil {
	return err
}
```

For multi-instance deployments, replace the default in-memory `Lockout` with a Redis or database-backed implementation so login failures are consistent across processes. `APITokenStore` should also be shared when API tokens are accepted by more than one process.

Direct `Service.Login` callers must provide a non-empty `LoginRequest.LockoutKey`. The HTTP handler derives it from the client IP and a username hash.

## Principals

Successful user authentication returns an `auth.Principal` with:

- `Subject`: user ID
- `Username`: username
- `Role`: current role loaded from `UserStore`
- `Scopes`: current scopes loaded from `UserStore`
- `Kind`: `auth.PrincipalKindUser`

`AuthenticateBearer` reloads the user on every accepted access token, so disabled users, role changes, and scope changes take effect without waiting for the access token to expire.

API tokens authenticate to `auth.PrincipalKindAPIToken`. They carry their own role and scopes from `APITokenStore`.

## Boundaries

- Login returns `ok=false` for invalid credentials and disabled users.
- Store or verifier failures return `error`.
- Refresh validates the refresh token, reloads the current user, then rotates the refresh token.
- Logout revokes the refresh session.
- API tokens are generated once, returned as plaintext once, loaded by hash, and marked used only after the service accepts them.
- Hooks may be called concurrently and must not retain `context.Context` after returning.

Use `auth/rbac/http` when the same service should be exposed through JSON bearer HTTP routes and role/scope middleware.
