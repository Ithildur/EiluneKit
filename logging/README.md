# logging

- `New` returns `*slog.Logger`.
- `Helper` wraps `*slog.Logger`.
- Public package boundaries use `*slog.Logger`.
- App code may use `Helper`.
