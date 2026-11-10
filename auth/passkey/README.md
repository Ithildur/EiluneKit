# Passkeys

[中文](README_CN.md)

`passkey.Service` registers and verifies WebAuthn credentials. It returns a verified identity; existing auth services issue sessions and apply RBAC. It has no password dependency and does not generate private keys, synchronize accounts, or assign permissions.

Registration requests discoverable credentials and requires user verification. The algorithm list is ML-DSA-44 (`-48`), then ES256 (`-7`). This is negotiation, not a hybrid signature: each credential keeps its registered algorithm. Login uses its stored public key. Google Password Manager and other providers use the same protocol. The browser/user chooses the provider; the server cannot force Google storage or promise its ML-DSA support. Accounts allowing classical credentials or password recovery are not wholly quantum-resistant.

## Setup

The application supplies durable credentials and a lookup that rejects missing/disabled users. User IDs must be immutable, opaque, non-empty, free of surrounding whitespace, and never reassigned. Do not use an email address or username as the ID. WebAuthn user handles are stable SHA-256 values derived from the RP ID and account ID; changing either breaks existing credentials.

```go
keys, err := passkey.New(passkey.Options{
    RPID:          "example.com",
    RPDisplayName: "Example",
    Origins:       []string{"https://example.com"},
    Store:         credentialStore,
    Flows:         passkey.NewMemoryFlowStore(10000),
    LookupUser:    lookupActiveUser,
})
if err != nil {
    return err
}
```

`LookupUser` has signature `func(context.Context, string) (passkey.User, bool, error)`. Return `false, nil` for absent or disabled accounts; return an error for backend failures. Supply the current `Name` and `DisplayName` for the account picker. Lookup runs for registration, credential management, and after successful signature verification before login completes.

Use an RP-scoped store and flow namespace. Origins are explicit; do not derive them from untrusted request headers. The default ceremony lifetime is five minutes. Memory flows are bounded, process-local, and lost on restart; no background goroutines need closing. At capacity, new flows fail rather than evict live challenges. For multiple instances:

```go
flows, err := redisflow.New(redisClient, "example.com:passkey:flow:")
if err != nil {
    return err
}
// Set passkey.Options.Flows to flows.
```

Redis 6.2+ and `ContextTimeoutEnabled: true` are required. The caller owns the client. Operations have a five-second timeout. Redis outages are errors, with no memory fallback. Consumption uses `GETDEL`; a failed finish requires a new begin, including when a backend fails after consumption. A timeout may leave the flow already consumed.

## Choose the session integration

Cookie-backed auth:

```go
handler, err := authhttp.NewHandler(manager, authhttp.Options{
    LoginAuthenticator: existingPasswordAuthenticator,
    Passkeys: keys,
})
```

`login.finish` issues the same session tokens as password login, sets refresh/CSRF cookies, and returns `access_token`, `expires_at`, and `csrf_token`. The existing login event runs before the response; failure revokes the issued refresh session. Its `Username` is empty for passkeys; use `UserID` as the identity.

RBAC:

```go
// Build the existing rbac.Service first.
keys, err := passkey.New(passkey.Options{
    RPID: "example.com", RPDisplayName: "Example",
    Origins: []string{"https://example.com"},
    Store: credentialStore, Flows: flows,
    LookupUser: rbacService.PasskeyUser,
})
if err != nil {
    return err
}
handler, err := rbachttp.NewHandler(rbacService, rbachttp.Options{Passkeys: keys})
```

RBAC reloads the active user before issuing tokens and runs the existing success event. The response remains the RBAC login JSON, including refresh token, principal and session metadata. Roles/scopes come from the current user store, not the credential. API-token principals cannot bind or manage user passkeys.

Enable either integration independently. Both existing constructors retain their password dependency requirements, but passkey requests never call password verification. `Passkeys: nil` adds no routes. Applications with their own session issuance can use `BeginRegistration`, `FinishRegistration`, `BeginLogin`, and `FinishLogin` directly; they must authorize the user ID for registration and management. Do not expose an endpoint that treats a caller-supplied user ID as a verified identity.

## HTTP contract

Paths follow the existing `BasePath` (default `/auth`). `POST /auth/passkey` takes JSON with `operation`:

| Operation | Other fields | Authentication | Success body |
|---|---|---|---|
| `register.begin` | none | User Bearer | `flow_id`, `options.publicKey` |
| `register.finish` | `flow_id`, `name`, `credential` | Same user Bearer | Credential metadata |
| `login.begin` | `persistence`: `session` or `persistent` | Public | `flow_id`, `options.publicKey` |
| `login.finish` | `flow_id`, `credential` | Public | Adapter's existing login response |

`credential` is the object returned by `PublicKeyCredential.toJSON()`. Persistence is bound at begin; sending another value at finish does not change it. Flows are single-use and bound to their operation, RP and registration user. Names contain 1–100 Unicode characters after trimming.

`GET /auth/passkeys` returns `{"credentials": [...]}`. `PATCH /auth/passkeys/{id}` accepts `{"name":"Phone"}`; `DELETE /auth/passkeys/{id}` removes the credential. Both mutations return 204. All management operations require the owning user's Bearer token. IDs are unpadded base64url. Metadata contains `id`, `name`, `created_at`, and optional `last_used_at`; it excludes keys. Deleting a credential does not revoke established sessions. Compose with existing session revocation if required.

The ceremony endpoint uses the handler's body limit and a separate limiter with the same `RateLimit` options. Both begin and finish count as requests. Password lockout and CAPTCHA continue to protect password login; passkey traffic does not increment or clear password lockout state. Invalid assertions, inactive accounts, and consumed flows return 401; missing owned credentials return 404; duplicate registration returns 409; backend errors remain server errors. The mixed ceremony route's generated contract describes operation-specific authentication in its summary; registration enforces Bearer authentication at runtime.

## Persistence and browser example

[`examples/passkey/postgres.go`](../../examples/passkey/postgres.go) and [`schema.sql`](../../examples/passkey/schema.sql) show an application-owned PostgreSQL implementation. This is an example, not an automatically installed migration or required schema. Add the application's user foreign key and account-deletion policy as appropriate.

- Enforce uniqueness on `(rp_id, id)`. Insert must never overwrite or transfer ownership.
- Persist the complete `webauthn.Credential`, including public key, flags, authenticator state and extensions. Keep durable data separate from short-lived flows.
- Usage updates must match credential ID and user ID, update only an existing record, and preserve keys and names. Never use an upsert. Preserve the maximum counter and latched verification state when concurrent requests complete out of order. Synchronized credentials may have zero counters; kit follows library counter/backup semantics instead of imposing strict monotonic rejection.
- Propagate database failures. Login does not issue a session until the usage update succeeds.

[`client.js`](../../examples/passkey/client.js) uses browser JSON conversion APIs, without provider detection or algorithm branches. Serve it from the configured HTTPS origin (localhost is supported for development), call `registerPasskey(accessToken, name)` for binding, and `loginWithPasskey()` for login. Consume the returned session using the selected auth adapter's conventions. Account signup, recovery, a management UI and Google synchronization are application/provider responsibilities.
