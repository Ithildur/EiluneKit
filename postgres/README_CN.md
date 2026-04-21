# postgres

Postgres 专用辅助包。各子包彼此独立。

## 子包

- `dbtypes`：薄数据库类型别名，例如 `TextArray`
- `gorm`：Postgres DSN、`*gorm.DB` 构造和 ping 辅助
- `pgx`：Postgres DSN、`*pgxpool.Pool` 构造和 ping 辅助

## 说明

- `gorm` 和 `pgx` 需要显式提供非空 `context.Context`
- `dbtypes` 用来把驱动相关类型别名隔离在业务模型包之外
