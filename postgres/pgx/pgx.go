// Package pgx provides pgx pool helpers for Postgres.
// Package pgx 提供 Postgres 的 pgx 连接池辅助函数。
package pgx

import (
	"context"
	"fmt"
	"time"

	"github.com/Ithildur/EiluneKit/contextutil"
	pgdsn "github.com/Ithildur/EiluneKit/postgres/internal/dsn"

	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultConnectTimeout = 5 * time.Second

// Config configures NewPool.
// Config 配置 NewPool。
type Config struct {
	Host            string
	Port            int
	User            string
	Password        string
	Database        string
	SSLMode         string
	MaxConns        int
	MinConns        int
	MaxConnLifetime time.Duration
	// ConnectTimeout limits each connection attempt. Non-positive values use five seconds.
	// ConnectTimeout 限制每次连接尝试；非正值使用五秒。
	ConnectTimeout time.Duration
}

// BuildDSN returns the Postgres DSN for cfg.
// BuildDSN 返回 cfg 的 Postgres DSN。
func BuildDSN(cfg Config) (string, error) {
	return pgdsn.Build(pgdsn.Config{
		Host:     cfg.Host,
		Port:     cfg.Port,
		User:     cfg.User,
		Password: cfg.Password,
		Database: cfg.Database,
		SSLMode:  cfg.SSLMode,
	})
}

// NewPool returns a pgx connection pool.
// Call NewPool(ctx, Config{...}).
// NewPool 返回 pgx 连接池。
// 调用 NewPool(ctx, Config{...})。
//
// Example / 示例:
//
//	pool, _ := pgx.NewPool(ctx, pgx.Config{Host: "localhost", Port: 5432, User: "app", Database: "db"})
func NewPool(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	ctx = contextutil.Require(ctx)

	dsn, err := BuildDSN(cfg)
	if err != nil {
		return nil, err
	}
	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse pg config: %w", err)
	}

	if cfg.MaxConns > 0 {
		poolCfg.MaxConns = int32(cfg.MaxConns)
	}
	if cfg.MinConns > 0 {
		poolCfg.MinConns = int32(cfg.MinConns)
	}
	if cfg.MaxConnLifetime > 0 {
		poolCfg.MaxConnLifetime = cfg.MaxConnLifetime
	}

	poolCfg.ConnConfig.ConnectTimeout = cfg.ConnectTimeout
	if poolCfg.ConnConfig.ConnectTimeout <= 0 {
		poolCfg.ConnConfig.ConnectTimeout = defaultConnectTimeout
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	return pool, nil
}

// Ping checks pool connectivity.
// Ping 检查连接池连通性。
func Ping(ctx context.Context, pool *pgxpool.Pool) error {
	ctx = contextutil.Require(ctx)
	if pool == nil {
		return fmt.Errorf("pgx pool is nil")
	}
	return pool.Ping(ctx)
}
