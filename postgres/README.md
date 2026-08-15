# postgres

Postgres-specific helpers. Each subpackage is independent.

## Packages

- `dbtypes`: thin database type aliases such as `TextArray`
- `gorm`: Postgres DSN construction, `*gorm.DB` setup, and ping helpers
- `migration`: explicit Goose-backed migrations with PostgreSQL advisory locking
- `pgx`: Postgres DSN construction, `*pgxpool.Pool` setup, and ping helpers

## Migrations

The application owns its migration sources and database lifecycle. Expose migration files at the root of an `fs.FS`, usually with `fs.Sub`, then call `migration.Run` from the application's explicit migration command:

```go
source, err := fs.Sub(embedded, "migrations")
if err != nil {
	return err
}

result, err := migration.Run(ctx, migration.Config{
	DB:         sqlDB,
	Migrations: source,
	LockID:     749153421,
})
if err != nil {
	return err
}
fmt.Printf("migrate: total=%d applied=%d skipped=%d\n", result.Total, result.Applied, result.Skipped)
```

`Run` applies pending migrations upward and rejects a database version newer than the available sources. It uses Goose's `goose_db_version` table, disables Goose's global Go-migration registry, and serializes migration runs with a PostgreSQL session advisory lock. A zero `LockID` uses Goose's default lock ID.

Applications that keep Go migrations next to SQL migrations pass migrations built with `goose.NewGoMigration` through `Config.GoMigrations`.

Normal startup can reject pending migrations and databases newer than the available sources without applying application migrations. Pass the same SQL and Go migration sources used by the migration command:

```go
if err := migration.RequireCurrent(ctx, cfg); err != nil {
	return err
}
```

Use `errors.Is` with `migration.ErrSchemaAhead` and `migration.ErrSchemaBehind` when startup needs different diagnostics. Goose may initialize its `goose_db_version` tracking table when either function first inspects a new database. Neither function closes the supplied `*sql.DB`. Database connection helpers never run migrations implicitly.

## Notes

- `gorm` and `pgx` expect an explicit non-nil `context.Context`
- `gorm.NewLogger` hides SQL query parameter values by default; set `LogOptions.IncludeQueryParams` only for controlled debugging
- `migration` expects an explicit non-nil `context.Context`; the application command remains responsible for configuration, connection setup, output, and exit status
- `dbtypes` keeps driver-specific aliases out of application model packages
