// Package gorm provides GORM helpers for Postgres.
// Package gorm 提供 Postgres 的 GORM 辅助函数。
package gorm

import (
	"context"
	"fmt"
	"time"

	"github.com/Ithildur/EiluneKit/contextutil"
	pgdsn "github.com/Ithildur/EiluneKit/postgres/internal/dsn"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const defaultPingTimeout = 5 * time.Second

// Config configures Connect.
// Config 配置 Connect。
type Config struct {
	Host                 string
	Port                 int
	User                 string
	Password             string
	Name                 string
	SSLMode              string
	MaxOpenConns         int
	MaxIdleConns         int
	ConnMaxLifetime      time.Duration
	PreferSimpleProtocol bool
	Logger               logger.Interface
}

// BuildDSN returns the Postgres DSN for cfg.
// BuildDSN 返回 cfg 的 Postgres DSN。
func BuildDSN(cfg Config) (string, error) {
	return pgdsn.Build(pgdsn.Config{
		Host:     cfg.Host,
		Port:     cfg.Port,
		User:     cfg.User,
		Password: cfg.Password,
		Name:     cfg.Name,
		SSLMode:  cfg.SSLMode,
	})
}

// Connect opens and verifies a Postgres-backed GORM DB.
// Call Connect(ctx, Config{...}).
// Connect 打开并验证基于 Postgres 的 GORM DB。
// 调用 Connect(ctx, Config{...})。
//
// Example / 示例:
//
//	db, _ := gorm.Connect(ctx, gorm.Config{Host: "localhost", Port: 5432, User: "app", Name: "db"})
func Connect(ctx context.Context, cfg Config) (*gorm.DB, error) {
	dsn, err := BuildDSN(cfg)
	if err != nil {
		return nil, err
	}

	gormCfg := &gorm.Config{}
	if cfg.Logger != nil {
		gormCfg.Logger = cfg.Logger
	}

	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  dsn,
		PreferSimpleProtocol: cfg.PreferSimpleProtocol,
	}), gormCfg)
	if err != nil {
		return nil, fmt.Errorf("open gorm postgres: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("extract sql.DB: %w", err)
	}

	if cfg.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	}

	if err := Ping(ctx, db); err != nil {
		return nil, err
	}

	return db, nil
}

// Ping checks connectivity through db.DB().
// Ping 通过 db.DB() 检查连通性。
func Ping(ctx context.Context, db *gorm.DB) error {
	pingCtx := contextutil.Require(ctx)
	if db == nil {
		return fmt.Errorf("gorm db is nil")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("extract sql.DB: %w", err)
	}

	if _, ok := pingCtx.Deadline(); !ok {
		var cancel context.CancelFunc
		pingCtx, cancel = context.WithTimeout(pingCtx, defaultPingTimeout)
		defer cancel()
	}

	return sqlDB.PingContext(pingCtx)
}
