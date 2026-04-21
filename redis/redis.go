// Package redis provides Redis client helpers.
// Package redis 提供 Redis 客户端辅助函数。
package redis

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Ithildur/EiluneKit/contextutil"
	"github.com/redis/go-redis/v9"
)

// Config configures NewClient.
// Config 配置 NewClient。
type Config struct {
	Addr         string
	Username     string
	Password     string
	DB           int
	PoolSize     int
	MinIdleConns int
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// NewClient returns a Redis client.
// Call NewClient(Config{Addr: "host:port"}).
// NewClient 返回 Redis client。
// 调用 NewClient(Config{Addr: "host:port"})。
//
// Example / 示例:
//
//	client, _ := redis.NewClient(redis.Config{Addr: "localhost:6379"})
func NewClient(cfg Config) (*redis.Client, error) {
	addr := strings.TrimSpace(cfg.Addr)
	if addr == "" {
		return nil, fmt.Errorf("redis addr is required")
	}

	opts := &redis.Options{
		Addr:         addr,
		Username:     cfg.Username,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
	}
	if cfg.DialTimeout > 0 {
		opts.DialTimeout = cfg.DialTimeout
	}
	if cfg.ReadTimeout > 0 {
		opts.ReadTimeout = cfg.ReadTimeout
	}
	if cfg.WriteTimeout > 0 {
		opts.WriteTimeout = cfg.WriteTimeout
	}

	return redis.NewClient(opts), nil
}

// Ping checks Redis connectivity.
// Ping 检查 Redis 连通性。
func Ping(ctx context.Context, client *redis.Client) error {
	ctx = contextutil.Require(ctx)
	if client == nil {
		return fmt.Errorf("redis client is nil")
	}
	return client.Ping(ctx).Err()
}
