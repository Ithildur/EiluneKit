# http/middleware

[中文](README_CN.md)

Optional standard `func(http.Handler) http.Handler` middleware. Inject these into `routes.HandlerOptions.Middleware`, attach endpoint-only policies through Blueprint, or use them with any `net/http` router. Constructors do not install themselves.

## Application middleware

```go
handler, err := routes.NewHandler(root.Routes(), routes.HandlerOptions{
	Middleware: []routes.Middleware{
		middleware.RequestID,
		middleware.AccessLog(middleware.AccessLogOptions{Logger: logger}),
		middleware.CORS(middleware.CORSOptions{
			AllowedOrigins:   []string{"https://app.example.com"},
			AllowedMethods:   []string{"GET", "POST", "DELETE"},
			AllowedHeaders:   []string{"Authorization", "Content-Type"},
			AllowCredentials: true,
			MaxAge:           5 * time.Minute,
		}),
		middleware.Compress(middleware.CompressOptions{MinSize: 500}),
		middleware.Recover(middleware.RecoverOptions{Logger: logger}),
	},
})
if err != nil {
	return err
}
```

The list runs from top to bottom on entry and in reverse on exit. Request IDs precede logging; recovery inside compression lets panic responses use the same encoder. An outer middleware's own panic is outside an inner `Recover`. CORS preflight requests may finish before reaching compression, recovery, or endpoints. Put policies that must cover preflight before CORS.

## Request IDs and logging

`RequestID` delegates to chi's request ID middleware. It accepts the incoming `X-Request-Id` (or chi's configured request ID header), otherwise generates an ID. It sets context only, not a response header. Read it with `RequestIDFromContext(ctx)`; missing or nil context returns `""`. Incoming IDs are correlation labels, not trusted identities.

`AccessLog` uses the same context and logs status, elapsed milliseconds, method, path, and client details through `slog`. A nil logger disables access logging. `Skip` and `MinLevel` control output; forwarded client addresses require configured trusted proxies.

Access logs are also emitted when a handler exits through panic, including `http.ErrAbortHandler`. These records use error level and include `aborted: true`. The status is the observed final status, or `0` if no final status was observed; a panic does not invent a 500 or a 200. Ordinary responses keep their existing fields and levels. `Skip` and `MinLevel` apply to aborted records too; `Skip` may therefore receive status `0`. Panics propagate unchanged.

## Panic recovery

`Recover(RecoverOptions{Logger: logger, OnPanic: failureHandler})` logs the panic value and stack. Nil `Logger` selects `slog.Default()` at construction. Nil `OnPanic` returns a plain-text 500 without panic details. A custom handler owns its response status and body.

Use `OnPanic: response.InternalServerError()` for a JSON 500 response. Presets and custom handler examples are in [http/response](../response/README.md).

Before invoking `OnPanic`, recovery clears the pending `Content-Length` so the failure body can have a different size. Other headers remain available to the handler.

Recovery covers the serving goroutine only. Once the response has started, it re-panics with `http.ErrAbortHandler` to abort the request instead of appending an error body. It closes a hijacked connection on panic and passes through `http.ErrAbortHandler` without logging. Flushing, connection hijacking, and response-controller capabilities remain available through the wrapper. Recovery does not buffer responses.

## CORS

`CORS(CORSOptions{...})` uses `rs/cors` and handles preflight before routing, including for unregistered paths. It adds permission headers to ordinary responses, including errors. A disallowed origin does not receive permission headers; CORS does not authenticate requests or prevent non-browser clients from reaching handlers.

| Option | Default and behavior |
|---|---|
| `AllowedOrigins` | Empty denies all origins. Exact origins and patterns containing one `*` are supported. Explicit `"*"` allows every origin. |
| `AllowedMethods` | GET, HEAD, POST. List methods explicitly. |
| `AllowedHeaders` | Accept, Content-Type, X-Requested-With. `"*"` allows all requested headers. |
| `ExposedHeaders` | No additional exposed headers. |
| `AllowCredentials` | False. Combining true with a `"*"` origin panics at construction. |
| `MaxAge` | Zero leaves the browser's preflight cache default. Positive durations are truncated to seconds; negative durations panic. |

Configuration slices are copied during construction. No dynamic configuration registry or global CORS policy is installed.

## Compression

`Compress(CompressOptions{...})` uses `klauspost/compress/gzhttp` for gzip response compression. `Level` accepts -2, -1, or 1–9; zero chooses the default compression level. `MinSize` is in bytes; zero chooses 1024. Negative sizes and invalid levels panic during construction.

`ContentTypes` optionally restricts compression to explicit MIME types. Empty uses gzhttp's default filter, which excludes common compressed audio, video, and archive formats. Invalid MIME types and wildcards panic. Parameters are supported: `text/plain` also matches `text/plain; charset=utf-8`.

Compression respects gzip negotiation, skips HEAD, already encoded responses, and content ranges, and adds `Vary: Accept-Encoding`. Compressed responses lose their original `ETag` and `Content-Length`. Zstd and request decompression are not enabled. Only the initial threshold/detection buffer is retained, rather than the entire response; flushing and connection hijacking are supported.

## Other middleware

- `LimitBody(maxBytes)` wraps the body with `http.MaxBytesReader` when the limit is positive. It does not pre-read or automatically return 413; the body consumer handles the error.
- `RequireJSONBody` requires `application/json` for a nonempty body. Valid media-type parameters and whitespace are accepted; malformed parameters receive 415. Attach it to endpoints that decode JSON.
- `RateLimit(RateLimitOptions{...})` accepts an application key function and `OnLimit` response callback. IP keys can use configured trusted proxies.

API prefixes, cache policies, body limits, and error formats belong to the application. Inject failure handlers through `routes.HandlerOptions`; `routes.NewHandler` sets `Allow` before invoking the application's 405 handler. See [complete HTTP handler](../routes/README.md#complete-http-handler) for boundary injection and SPA fallback.
