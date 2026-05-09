package store

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Ithildur/EiluneKit/contextutil"

	"github.com/redis/go-redis/v9"
)

const (
	defaultReadTimeout  = 3 * time.Second
	defaultWriteTimeout = 3 * time.Second
)

const (
	sessionFieldUserID      = "user_id"
	sessionFieldRefreshID   = "refresh_id"
	sessionFieldExpiresAt   = "expires_at"
	sessionFieldSessionOnly = "session_only"
)

// RedisStore stores sessions in Redis.
// RedisStore 在 Redis 中保存 session。
type RedisStore struct {
	client       *redis.Client
	prefix       string
	writeTimeout time.Duration
	readTimeout  time.Duration
}

var _ SessionStore = (*RedisStore)(nil)

// RedisOptions configures NewRedisStore.
// RedisOptions 配置 NewRedisStore。
type RedisOptions struct {
	Prefix       string
	WriteTimeout time.Duration
	ReadTimeout  time.Duration
}

//go:embed scripts/rotate_refresh.lua
var rotateRefreshLua string

var rotateRefreshScript = redis.NewScript(rotateRefreshLua)

// NewRedisStore returns a Redis-backed SessionStore.
// NewRedisStore 返回 Redis 版 SessionStore。
func NewRedisStore(client *redis.Client, opts RedisOptions) *RedisStore {
	prefix := opts.Prefix
	if prefix == "" {
		prefix = "auth:jwt:"
	}
	readTimeout := opts.ReadTimeout
	if readTimeout <= 0 {
		readTimeout = defaultReadTimeout
	}
	writeTimeout := opts.WriteTimeout
	if writeTimeout <= 0 {
		writeTimeout = defaultWriteTimeout
	}
	return &RedisStore{
		client:       client,
		prefix:       prefix,
		writeTimeout: writeTimeout,
		readTimeout:  readTimeout,
	}
}

// UserVersion returns the current version for userID.
// UserVersion 返回 userID 的当前版本。
func (s *RedisStore) UserVersion(ctx context.Context, userID string) (int64, error) {
	ctx = contextutil.Require(ctx)
	if s == nil || s.client == nil {
		return 0, ErrStoreUnavailable
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return 0, nil
	}
	ctx, cancel := context.WithTimeout(ctx, s.readTimeout)
	defer cancel()
	val, err := s.client.Get(ctx, s.userVersionKey(userID)).Result()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, ErrStoreUnavailable
	}
	version, convErr := strconv.ParseInt(val, 10, 64)
	if convErr != nil {
		return 0, fmt.Errorf("invalid user version %q", val)
	}
	return version, nil
}

// BumpUserVersion invalidates all sessions for userID.
// BumpUserVersion 通过提升版本使 userID 的全部 session 失效。
func (s *RedisStore) BumpUserVersion(ctx context.Context, userID string) (int64, error) {
	ctx = contextutil.Require(ctx)
	if s == nil || s.client == nil {
		return 0, ErrStoreUnavailable
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return 0, nil
	}
	ctx, cancel := context.WithTimeout(ctx, s.writeTimeout)
	defer cancel()
	version, err := s.client.Incr(ctx, s.userVersionKey(userID)).Result()
	if err != nil {
		return 0, ErrStoreUnavailable
	}
	return version, nil
}

// CreateSession stores a session.
// CreateSession 保存 session。
func (s *RedisStore) CreateSession(ctx context.Context, sessionID string, state SessionState) error {
	ctx = contextutil.Require(ctx)
	if s == nil || s.client == nil {
		return ErrStoreUnavailable
	}
	sessionID = strings.TrimSpace(sessionID)
	state.UserID = strings.TrimSpace(state.UserID)
	state.RefreshID = strings.TrimSpace(state.RefreshID)
	if sessionID == "" || state.UserID == "" || state.RefreshID == "" || state.ExpiresAt.IsZero() {
		return errors.New("session state is incomplete")
	}
	ttl := time.Until(state.ExpiresAt)
	if ttl <= 0 {
		return errors.New("session already expired")
	}
	ctx, cancel := context.WithTimeout(ctx, s.writeTimeout)
	defer cancel()
	pipe := s.client.TxPipeline()
	pipe.HSet(ctx, s.sessionKey(sessionID), map[string]any{
		sessionFieldUserID:      state.UserID,
		sessionFieldRefreshID:   state.RefreshID,
		sessionFieldExpiresAt:   strconv.FormatInt(state.ExpiresAt.UTC().Unix(), 10),
		sessionFieldSessionOnly: strconv.FormatBool(state.SessionOnly),
	})
	pipe.PExpire(ctx, s.sessionKey(sessionID), ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return ErrStoreUnavailable
	}
	return nil
}

// Session returns the session when still active.
// Session 返回仍然活跃的 session。
func (s *RedisStore) Session(ctx context.Context, sessionID string) (SessionState, bool, error) {
	ctx = contextutil.Require(ctx)
	if s == nil || s.client == nil {
		return SessionState{}, false, ErrStoreUnavailable
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return SessionState{}, false, nil
	}
	ctx, cancel := context.WithTimeout(ctx, s.readTimeout)
	defer cancel()
	values, err := s.client.HGetAll(ctx, s.sessionKey(sessionID)).Result()
	if err != nil {
		return SessionState{}, false, ErrStoreUnavailable
	}
	if len(values) == 0 {
		return SessionState{}, false, nil
	}
	expUnix, convErr := strconv.ParseInt(values[sessionFieldExpiresAt], 10, 64)
	if convErr != nil {
		return SessionState{}, false, fmt.Errorf("invalid session expiration %q", values[sessionFieldExpiresAt])
	}
	state := SessionState{
		UserID:      values[sessionFieldUserID],
		RefreshID:   values[sessionFieldRefreshID],
		ExpiresAt:   time.Unix(expUnix, 0).UTC(),
		SessionOnly: strings.EqualFold(values[sessionFieldSessionOnly], "true"),
	}
	if state.UserID == "" || state.RefreshID == "" {
		return SessionState{}, false, nil
	}
	return state, true, nil
}

// RotateRefresh replaces the refresh state for a session.
// RotateRefresh 替换某个 session 的 refresh 状态。
func (s *RedisStore) RotateRefresh(ctx context.Context, sessionID, userID string, expectedVersion int64, oldRefreshID, newRefreshID string, exp time.Time) (bool, error) {
	ctx = contextutil.Require(ctx)
	if s == nil || s.client == nil {
		return false, ErrStoreUnavailable
	}
	sessionID = strings.TrimSpace(sessionID)
	userID = strings.TrimSpace(userID)
	oldRefreshID = strings.TrimSpace(oldRefreshID)
	newRefreshID = strings.TrimSpace(newRefreshID)
	if sessionID == "" || userID == "" || oldRefreshID == "" || newRefreshID == "" || exp.IsZero() {
		return false, nil
	}
	ttl := time.Until(exp)
	if ttl <= 0 {
		return false, errors.New("session already expired")
	}
	ctx, cancel := context.WithTimeout(ctx, s.writeTimeout)
	defer cancel()
	res, err := rotateRefreshScript.Run(
		ctx,
		s.client,
		[]string{s.userVersionKey(userID), s.sessionKey(sessionID)},
		expectedVersion,
		userID,
		oldRefreshID,
		newRefreshID,
		exp.UTC().Unix(),
		ttl.Milliseconds(),
	).Result()
	if err != nil {
		return false, ErrStoreUnavailable
	}
	return scriptBoolResult(res)
}

// RevokeSession deletes a session.
// RevokeSession 删除 session。
func (s *RedisStore) RevokeSession(ctx context.Context, sessionID string) error {
	ctx = contextutil.Require(ctx)
	if s == nil || s.client == nil {
		return ErrStoreUnavailable
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, s.writeTimeout)
	defer cancel()
	if err := s.client.Del(ctx, s.sessionKey(sessionID)).Err(); err != nil {
		return ErrStoreUnavailable
	}
	return nil
}

func (s *RedisStore) userVersionKey(userID string) string {
	return s.prefix + "user:" + userID + ":version"
}

func (s *RedisStore) sessionKey(sessionID string) string {
	return s.prefix + "session:" + sessionID
}

func scriptBoolResult(res any) (bool, error) {
	switch v := res.(type) {
	case int64:
		return v == 1, nil
	case string:
		return v == "1", nil
	case []byte:
		return string(v) == "1", nil
	default:
		return false, fmt.Errorf("unexpected script result type %T", res)
	}
}
