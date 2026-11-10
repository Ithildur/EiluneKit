// Package redisflow stores single-use WebAuthn flows in Redis.
// Package redisflow 在 Redis 中存储一次性 WebAuthn 流程。
package redisflow

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Ithildur/EiluneKit/auth/passkey"
	"github.com/redis/go-redis/v9"
)

// Store uses Redis SET NX and GETDEL. The caller owns the client lifecycle.
// Store 使用 Redis SET NX 和 GETDEL。调用方负责客户端生命周期。
type Store struct {
	client *redis.Client
	prefix string
}

// New requires a Redis 6.2+ client and a dedicated, RP-scoped key prefix.
// Enable ContextTimeoutEnabled on the client; operations have a five-second limit.
// New 要求 Redis 6.2+ 客户端及专属的 RP 范围 key 前缀。
// 客户端须启用 ContextTimeoutEnabled；操作限时五秒。
func New(client *redis.Client, prefix string) (*Store, error) {
	if client == nil || strings.TrimSpace(prefix) == "" {
		return nil, fmt.Errorf("redisflow: client and prefix are required")
	}
	if !client.Options().ContextTimeoutEnabled {
		return nil, fmt.Errorf("redisflow: ContextTimeoutEnabled is required")
	}
	return &Store{client: client, prefix: prefix}, nil
}

// Create saves a flow with its remaining lifetime and rejects existing IDs.
// Create 按流程剩余寿命保存，并拒绝已有 ID。
func (s *Store) Create(ctx context.Context, id string, flow passkey.Flow) error {
	data, err := json.Marshal(flow)
	if err != nil {
		return err
	}
	ttl := time.Until(flow.Session.Expires)
	if ttl <= 0 {
		return passkey.ErrRejected
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ok, err := s.client.SetNX(ctx, s.prefix+id, data, ttl).Result()
	if err != nil {
		return fmt.Errorf("create redis flow: %w", err)
	}
	if !ok {
		return passkey.ErrConflict
	}
	return nil
}

// Consume atomically removes and returns a live flow.
// Consume 原子移除并返回未过期流程。
func (s *Store) Consume(ctx context.Context, id string) (passkey.Flow, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	data, err := s.client.GetDel(ctx, s.prefix+id).Bytes()
	if errors.Is(err, redis.Nil) {
		return passkey.Flow{}, false, nil
	}
	if err != nil {
		return passkey.Flow{}, false, fmt.Errorf("consume redis flow: %w", err)
	}
	var flow passkey.Flow
	if err = json.Unmarshal(data, &flow); err != nil {
		return passkey.Flow{}, false, fmt.Errorf("decode redis flow: %w", err)
	}
	if !flow.Session.Expires.After(time.Now()) {
		return passkey.Flow{}, false, nil
	}
	return flow, true, nil
}
