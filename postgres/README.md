# postgres

Postgres-specific helpers. Each subpackage is independent.

## Packages

- `dbtypes`: thin database type aliases such as `TextArray`
- `gorm`: Postgres DSN construction, `*gorm.DB` setup, and ping helpers
- `pgx`: Postgres DSN construction, `*pgxpool.Pool` setup, and ping helpers

## Notes

- `gorm` and `pgx` expect an explicit non-nil `context.Context`
- `gorm.NewLogger` hides SQL query parameter values by default; set `LogOptions.IncludeQueryParams` only for controlled debugging
- `dbtypes` keeps driver-specific aliases out of application model packages
