// Package migration provides Goose-backed PostgreSQL schema migration helpers.
// Package migration 提供基于 Goose 的 PostgreSQL schema 迁移辅助函数。
package migration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"

	"github.com/Ithildur/EiluneKit/contextutil"

	"github.com/pressly/goose/v3"
	gooselock "github.com/pressly/goose/v3/lock"
)

var (
	// ErrSchemaAhead reports that the database was migrated beyond the available sources.
	// ErrSchemaAhead 表示数据库版本高于当前可用的迁移源。
	ErrSchemaAhead = errors.New("database schema is newer than available migrations")
	// ErrSchemaBehind reports that migrations remain unapplied.
	// ErrSchemaBehind 表示仍有迁移尚未应用。
	ErrSchemaBehind = errors.New("database schema migrations are pending")
	// ErrTargetInvalid reports that a RunTo target is outside the upward migration range.
	// ErrTargetInvalid 表示 RunTo 的目标超出向上迁移范围。
	ErrTargetInvalid = errors.New("invalid migration target")
)

// Config configures Run, RunTo, and RequireCurrent. DB remains owned by the caller.
// A zero LockID uses Goose's default PostgreSQL advisory lock ID.
// Config 配置 Run、RunTo 和 RequireCurrent。DB 的生命周期仍由调用方管理。
// LockID 为零时使用 Goose 默认的 PostgreSQL advisory lock ID。
type Config struct {
	DB           *sql.DB
	Migrations   fs.FS
	LockID       int64
	GoMigrations []*goose.Migration
}

// Result summarizes a successful upward migration run.
// RunTo excludes sources newer than its target from Total and Skipped.
// Result 汇总一次成功的向上迁移执行结果。
// RunTo 的 Total 和 Skipped 不包含高于目标版本的迁移源。
type Result struct {
	Total   int
	Applied int
	Skipped int
}

// Run applies all pending migrations. It rejects a database newer than the available sources.
// Run 应用全部待执行迁移；数据库版本高于可用迁移源时拒绝执行。
func Run(ctx context.Context, cfg Config) (Result, error) {
	ctx = contextutil.Require(ctx)
	provider, err := newProvider(cfg)
	if err != nil {
		return Result{}, err
	}

	total := len(provider.ListSources())
	current, target, err := provider.GetVersions(ctx)
	if err != nil {
		return Result{Total: total}, fmt.Errorf("read schema versions: %w", err)
	}
	if current > target {
		return Result{Total: total}, versionError(ErrSchemaAhead, current, target)
	}

	results, err := provider.Up(ctx)
	if err != nil {
		return Result{Total: total}, fmt.Errorf("run migrations: %w", err)
	}
	return Result{
		Total:   total,
		Applied: len(results),
		Skipped: total - len(results),
	}, nil
}

// RunTo applies pending migrations through target.
// The target cannot be older than the database or newer than the available migrations.
// RunTo is intended for upgrade tests and maintenance, not normal application startup.
// RunTo 应用截至 target 的待执行迁移。
// 目标不能低于数据库版本，也不能高于当前可用迁移。
// RunTo 用于升级测试和维护，不用于正常应用启动。
func RunTo(ctx context.Context, cfg Config, target int64) (Result, error) {
	ctx = contextutil.Require(ctx)
	if target < 1 {
		return Result{}, fmt.Errorf("%w: target=%d must be greater than zero", ErrTargetInvalid, target)
	}

	provider, err := newProvider(cfg)
	if err != nil {
		return Result{}, err
	}
	sources := provider.ListSources()
	current, available, err := provider.GetVersions(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("read schema versions: %w", err)
	}

	total, err := targetTotal(sources, current, available, target)
	if err != nil {
		return Result{}, err
	}
	results, err := provider.UpTo(ctx, target)
	if err != nil {
		return Result{Total: total}, fmt.Errorf("run migrations to %d: %w", target, err)
	}
	return Result{
		Total:   total,
		Applied: len(results),
		Skipped: total - len(results),
	}, nil
}

// RequireCurrent requires the database to have no pending migrations and not exceed the available version.
// RequireCurrent 要求数据库没有待执行迁移，且版本不高于当前可用版本。
func RequireCurrent(ctx context.Context, cfg Config) error {
	ctx = contextutil.Require(ctx)
	provider, err := newProvider(cfg)
	if err != nil {
		return err
	}

	current, target, err := provider.GetVersions(ctx)
	if err != nil {
		return fmt.Errorf("read schema versions: %w", err)
	}
	if current > target {
		return versionError(ErrSchemaAhead, current, target)
	}
	if current < target {
		return versionError(ErrSchemaBehind, current, target)
	}

	pending, err := provider.HasPending(ctx)
	if err != nil {
		return fmt.Errorf("inspect pending migrations: %w", err)
	}
	if pending {
		return versionError(ErrSchemaBehind, current, target)
	}
	return nil
}

func newProvider(cfg Config) (*goose.Provider, error) {
	if cfg.DB == nil {
		return nil, errors.New("migration db is nil")
	}

	lockOptions := make([]gooselock.SessionLockerOption, 0, 1)
	if cfg.LockID != 0 {
		lockOptions = append(lockOptions, gooselock.WithLockID(cfg.LockID))
	}
	locker, err := gooselock.NewPostgresSessionLocker(lockOptions...)
	if err != nil {
		return nil, fmt.Errorf("create migration locker: %w", err)
	}

	options := []goose.ProviderOption{
		goose.WithSessionLocker(locker),
		goose.WithDisableGlobalRegistry(true),
	}
	if len(cfg.GoMigrations) != 0 {
		options = append(options, goose.WithGoMigrations(cfg.GoMigrations...))
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, cfg.DB, cfg.Migrations, options...)
	if err != nil {
		return nil, fmt.Errorf("create migration provider: %w", err)
	}
	return provider, nil
}

func targetTotal(sources []*goose.Source, current, available, target int64) (int, error) {
	if current > available {
		return 0, versionError(ErrSchemaAhead, current, available)
	}
	if target < current {
		return 0, fmt.Errorf("%w: target=%d database=%d", ErrTargetInvalid, target, current)
	}
	if target > available {
		return 0, fmt.Errorf("%w: target=%d available=%d", ErrTargetInvalid, target, available)
	}

	total := 0
	for _, source := range sources {
		if source.Version > target {
			break
		}
		total++
	}
	return total, nil
}

func versionError(kind error, current, target int64) error {
	return fmt.Errorf("%w: database=%d available=%d", kind, current, target)
}
