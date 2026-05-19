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

var (
	_ SessionStore       = (*RedisStore)(nil)
	_ SessionLister      = (*RedisStore)(nil)
	_ UserSessionCleaner = (*RedisStore)(nil)
	_ SessionCleaner     = (*RedisStore)(nil)
)

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
	return s.userVersionWithContext(ctx, userID)
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
	pipe.ZAdd(ctx, s.userSessionsKey(state.UserID), redis.Z{
		Score:  float64(state.ExpiresAt.UTC().Unix()),
		Member: sessionID,
	})
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
	return s.sessionWithContext(ctx, sessionID)
}

func (s *RedisStore) sessionWithContext(ctx context.Context, sessionID string) (SessionState, bool, error) {
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
		[]string{s.userVersionKey(userID), s.sessionKey(sessionID), s.userSessionsKey(userID)},
		expectedVersion,
		userID,
		oldRefreshID,
		newRefreshID,
		exp.UTC().Unix(),
		ttl.Milliseconds(),
		sessionID,
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

	userID, err := s.client.HGet(ctx, s.sessionKey(sessionID), sessionFieldUserID).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return ErrStoreUnavailable
	}

	pipe := s.client.TxPipeline()
	pipe.Del(ctx, s.sessionKey(sessionID))
	if strings.TrimSpace(userID) != "" {
		pipe.ZRem(ctx, s.userSessionsKey(userID), sessionID)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return ErrStoreUnavailable
	}
	return nil
}

// Sessions returns stored, unexpired sessions for userID.
// Sessions 返回 userID 已保存且未过期的 session。
func (s *RedisStore) Sessions(ctx context.Context, userID string) ([]SessionInfo, error) {
	ctx = contextutil.Require(ctx)
	if s == nil || s.client == nil {
		return nil, ErrStoreUnavailable
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, nil
	}

	now := time.Now().UTC()
	ctx, cancel := context.WithTimeout(ctx, s.readTimeout)
	defer cancel()

	key := s.userSessionsKey(userID)
	if err := s.client.ZRemRangeByScore(ctx, key, "-inf", strconv.FormatInt(now.Unix(), 10)).Err(); err != nil {
		return nil, ErrStoreUnavailable
	}
	ids, err := s.client.ZRangeByScore(ctx, key, &redis.ZRangeBy{
		Min: "(" + strconv.FormatInt(now.Unix(), 10),
		Max: "+inf",
	}).Result()
	if err != nil {
		return nil, ErrStoreUnavailable
	}

	out := make([]SessionInfo, 0, len(ids))
	stale := make([]any, 0)
	for _, sessionID := range ids {
		state, ok, err := s.sessionWithContext(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		if !ok || state.UserID != userID {
			stale = append(stale, sessionID)
			continue
		}
		out = append(out, SessionInfo{
			ID:          sessionID,
			ExpiresAt:   state.ExpiresAt,
			SessionOnly: state.SessionOnly,
		})
	}
	if len(stale) > 0 {
		if err := s.client.ZRem(ctx, key, stale...).Err(); err != nil {
			return nil, ErrStoreUnavailable
		}
	}
	return out, nil
}

// ClearUserSessions removes stored sessions for userID.
// ClearUserSessions 清理 userID 已保存的 session。
func (s *RedisStore) ClearUserSessions(ctx context.Context, userID string) error {
	ctx = contextutil.Require(ctx)
	if s == nil || s.client == nil {
		return ErrStoreUnavailable
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, s.writeTimeout)
	defer cancel()
	if err := s.clearUserSessionsWithContext(ctx, userID); err != nil {
		return ErrStoreUnavailable
	}
	return nil
}

// ClearAllSessions removes all stored sessions.
// ClearAllSessions 清理全部已保存的 session。
func (s *RedisStore) ClearAllSessions(ctx context.Context) error {
	ctx = contextutil.Require(ctx)
	if s == nil || s.client == nil {
		return ErrStoreUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, s.writeTimeout)
	defer cancel()
	if err := s.deleteKeysByPattern(ctx, s.sessionKey("*")); err != nil {
		return ErrStoreUnavailable
	}
	if err := s.deleteKeysByPattern(ctx, s.userSessionsKey("*")); err != nil {
		return ErrStoreUnavailable
	}
	return nil
}

func (s *RedisStore) clearUserSessionsWithContext(ctx context.Context, userID string) error {
	key := s.userSessionsKey(userID)
	sessionIDs, err := s.client.ZRange(ctx, key, 0, -1).Result()
	if err != nil {
		return err
	}
	pipe := s.client.TxPipeline()
	if len(sessionIDs) > 0 {
		keys := make([]string, 0, len(sessionIDs))
		for _, sessionID := range sessionIDs {
			keys = append(keys, s.sessionKey(sessionID))
		}
		pipe.Del(ctx, keys...)
	}
	pipe.Del(ctx, key)
	_, err = pipe.Exec(ctx)
	return err
}

func (s *RedisStore) deleteKeysByPattern(ctx context.Context, pattern string) error {
	var cursor uint64
	for {
		keys, next, err := s.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			if err := s.client.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}
		cursor = next
		if cursor == 0 {
			return nil
		}
	}
}

func (s *RedisStore) userVersionKey(userID string) string {
	return s.prefix + "user:" + userID + ":version"
}

func (s *RedisStore) userVersionWithContext(ctx context.Context, userID string) (int64, error) {
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

func (s *RedisStore) userSessionsKey(userID string) string {
	return s.prefix + "user:" + userID + ":sessions"
}

func (s *RedisStore) sessionKey(sessionID string) string {
	return s.prefix + "sessions:" + sessionID
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
