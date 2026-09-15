# http/response

[简体中文](README_CN.md)

JSON writers and optional error handlers. Import `github.com/Ithildur/EiluneKit/http/response`.

## Error handlers

| Constructor | Status | Code | Message |
|---|---|---|---|
| `NotFound()` | 404 | `not_found` | resource not found |
| `MethodNotAllowed()` | 405 | `method_not_allowed` | method not allowed |
| `Unauthorized()` | 401 | `unauthorized` | authentication required |
| `InternalServerError()` | 500 | `internal_error` | internal server error |

Each constructor returns a standard `http.HandlerFunc`, writing `ErrorResponse` with `Content-Type: application/json; charset=utf-8`. Handlers do not select paths or install themselves. The router owns `Allow`; authentication-specific headers belong to the application.

Inject the presets when building an API handler:

```go
handler, err := routes.NewHandler(api.RoutesAt("/api"), routes.HandlerOptions{
    NotFound:         response.NotFound(),
    MethodNotAllowed: response.MethodNotAllowed(),
    Unauthorized:     response.Unauthorized(),
    Middleware: []routes.Middleware{
        middleware.Recover(middleware.RecoverOptions{
            Logger:  logger,
            OnPanic: response.InternalServerError(),
        }),
    },
})
if err != nil {
    return err
}
```

Here `api` is the application's Blueprint and `logger` is its `*slog.Logger`. `NewHandler` sets `Allow` before the 405 handler. `Recover` only invokes `OnPanic` before a response starts. Injecting presets does not change defaults on other handlers.

When sharing a server with a SPA, the application must select which paths receive JSON errors; see [SPA fallback](../static/README.md).

## Override a preset

Replace the corresponding injected handler. The remaining presets can stay in place:

```go
func unauthorized(w http.ResponseWriter, r *http.Request) {
    response.WriteJSONError(w, http.StatusUnauthorized,
        "session_expired", "please sign in again")
}

handler, err := routes.NewHandler(api.RoutesAt("/api"), routes.HandlerOptions{
    NotFound:         response.NotFound(),
    MethodNotAllowed: response.MethodNotAllowed(),
    Unauthorized:     http.HandlerFunc(unauthorized),
})
if err != nil {
    return err
}
```

This replacement belongs to this handler instance. It does not redefine a Kit function or mutate global configuration. Use `WriteJSON` with an application struct if the response shape also needs to change.

## Add a business error handler

Define an ordinary handler and attach it at the relevant application boundary. For example, an export quota response can be used as a rate-limit callback:

```go
func quotaExceeded(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Retry-After", "60")
    response.WriteJSONError(w, http.StatusTooManyRequests,
        "export_quota_exceeded", "try again in one minute")
}

exports := routes.NewBlueprint(routes.DefaultMiddleware(
    middleware.RateLimit(middleware.RateLimitOptions{
        Requests: 10,
        Window:   time.Minute,
        OnLimit:  quotaExceeded,
    }),
))
exports.Post("/exports", "Create export", createExport)
```

`createExport` is the application's endpoint. No registration with the response package is needed. A business handler can also call `quotaExceeded(w, r)` directly and return.

The [executable examples](example_test.go) exercise presets, a custom 401 response, and a new quota handler. Run them with `go test ./http/response -run Example -v`.
