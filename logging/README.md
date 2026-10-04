# logging

- `New` returns `*slog.Logger`.
- `Helper` wraps `*slog.Logger`.
- Public package boundaries use `*slog.Logger`.
- App code may use `Helper`.

Pass a non-nil context to each `Helper` logging call and to `Enabled`:

```go
logger := logging.NewHelper(logging.New(logging.Options{
	Level:     logging.LevelInfo,
	AddSource: true,
}))
logger = logger.With(logging.String("service", "accounts"))
logger.Info(ctx, "user saved", nil, logging.String("user_id", userID))
logger.Error(ctx, "save user failed", err, logging.String("user_id", userID))
```

Use the request context in handlers and `context.Background()` for startup logs.
The context reaches the underlying handler for filtering and output. A non-nil
error is appended as the `error` attribute. `With` keeps its own attribute slice.

`AddSource` includes the application call site in both text and JSON output.
Text output uses `source=file:line`; JSON output uses slog's source object.

Text output quotes messages containing non-printable characters and escapes unsafe
attribute keys and values. Group names are encoded as part of the complete key.
