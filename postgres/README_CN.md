# postgres

Postgres 专用辅助包。各子包彼此独立。

## 子包

- `dbtypes`：薄数据库类型别名，例如 `TextArray`
- `gorm`：Postgres DSN、`*gorm.DB` 构造和 ping 辅助
- `migration`：带 PostgreSQL advisory lock 的显式 Goose 迁移
- `pgx`：Postgres DSN、`*pgxpool.Pool` 构造和 ping 辅助

## 数据库迁移

应用拥有迁移源和数据库生命周期。通常先用 `fs.Sub` 让迁移文件位于一个 `fs.FS` 的根目录，再从应用的显式迁移命令调用 `migration.Run`：

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

`Run` 向上应用待执行迁移，并拒绝数据库版本高于当前可用迁移源的情况。它使用 Goose 的 `goose_db_version` 表，禁用 Goose 全局 Go migration registry，并通过 PostgreSQL session advisory lock 串行化迁移。`LockID` 为零时使用 Goose 默认锁 ID。

需要同时执行 Go migrations 时，应用使用 `goose.NewGoMigration` 构造迁移并通过 `Config.GoMigrations` 传入。

迁移升级测试和维护工具可以通过 `migration.RunTo(ctx, cfg, target)` 应用不高于历史版本上限的全部可用迁移。目标必须为正数，不能低于数据库版本，也不能高于最新可用迁移。`RunTo` 使用与 `Run` 相同的 advisory lock；高于目标版本的迁移不会计入 `Result.Total` 和 `Result.Skipped`。

`RunTo` 不是正常生产迁移入口。当前应用代码应使用 `Run`，并且只在 `RequireCurrent` 成功后启动；中间版本 schema 可能无法支持当前二进制。

正常启动可以在不执行应用迁移的情况下拒绝待执行迁移，以及版本高于当前迁移源的数据库。这里必须传入与迁移命令相同的 SQL 和 Go migration 源：

```go
if err := migration.RequireCurrent(ctx, cfg); err != nil {
	return err
}
```

调用方需要区分诊断时，使用 `errors.Is` 检查 `migration.ErrSchemaAhead`、`migration.ErrSchemaBehind` 和 `migration.ErrTargetInvalid`。这些函数首次检查新数据库时，Goose 可能初始化自己的 `goose_db_version` 追踪表。它们都不会关闭传入的 `*sql.DB`。数据库连接辅助函数不会隐式执行迁移。

## 说明

- `gorm` 和 `pgx` 需要显式提供非空 `context.Context`
- `gorm.NewLogger` 默认隐藏 SQL 查询参数值；只在受控调试时设置 `LogOptions.IncludeQueryParams`
- `migration` 需要显式提供非空 `context.Context`；应用命令仍负责配置、连接装配、输出和退出状态
- `dbtypes` 用来把驱动相关类型别名隔离在业务模型包之外
