// Package redissession provides Redis-backed auth session storage.
// Package redissession 提供 Redis 版认证 session 存储。
package redissession

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	authstore "github.com/Ithildur/EiluneKit/auth/store"
	"github.com/Ithildur/EiluneKit/contextutil"

	"github.com/redis/go-redis/v9"
)

const (
	defaultReadTimeout   = 3 * time.Second
	defaultWriteTimeout  = 3 * time.Second
	sessionIndexTTLGrace = time.Minute
)

const (
	sessionFieldUserID      = "user_id"
	sessionFieldRefreshID   = "refresh_id"
	sessionFieldExpiresAt   = "expires_at"
	sessionFieldSessionOnly = "session_only"
)

// Store stores sessions in Redis.
// Store 在 Redis 中保存 session。
type Store struct {
	client       *redis.Client
	prefix       string
	writeTimeout time.Duration
	readTimeout  time.Duration
}

var (
	_ authstore.SessionStore       = (*Store)(nil)
	_ authstore.SessionLister      = (*Store)(nil)
	_ authstore.UserSessionCleaner = (*Store)(nil)
	_ authstore.SessionCleaner     = (*Store)(nil)
)

// Options configures New.
// Options 配置 New。
type Options struct {
	Prefix       string
	WriteTimeout time.Duration
	ReadTimeout  time.Duration
}

//go:embed scripts/create_session.lua
var createSessionLua string

//go:embed scripts/revoke_session.lua
var revokeSessionLua string

//go:embed scripts/rotate_refresh.lua
var rotateRefreshLua string

//go:embed scripts/trim_session_index.lua
var trimSessionIndexLua string

var createSessionScript = redis.NewScript(createSessionLua)
var revokeSessionScript = redis.NewScript(revokeSessionLua)
var rotateRefreshScript = redis.NewScript(rotateRefreshLua)
var trimSessionIndexScript = redis.NewScript(trimSessionIndexLua)

// New returns a Redis-backed session store.
// New 返回 Redis 版 session store。
func New(client *redis.Client, opts Options) *Store {
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
	return &Store{
		client:       client,
		prefix:       prefix,
		writeTimeout: writeTimeout,
		readTimeout:  readTimeout,
	}
}

// UserVersion returns the current version for userID.
// UserVersion 返回 userID 的当前版本。
func (s *Store) UserVersion(ctx context.Context, userID string) (int64, error) {
	ctx = contextutil.Require(ctx)
	if s == nil || s.client == nil {
		return 0, authstore.ErrStoreUnavailable
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
func (s *Store) BumpUserVersion(ctx context.Context, userID string) (int64, error) {
	ctx = contextutil.Require(ctx)
	if s == nil || s.client == nil {
		return 0, authstore.ErrStoreUnavailable
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return 0, nil
	}
	ctx, cancel := context.WithTimeout(ctx, s.writeTimeout)
	defer cancel()
	version, err := s.client.Incr(ctx, s.userVersionKey(userID)).Result()
	if err != nil {
		return 0, authstore.ErrStoreUnavailable
	}
	return version, nil
}

// CreateSession stores a session.
// CreateSession 保存 session。
func (s *Store) CreateSession(ctx context.Context, sessionID string, state authstore.SessionState) error {
	ctx = contextutil.Require(ctx)
	if s == nil || s.client == nil {
		return authstore.ErrStoreUnavailable
	}
	sessionID = strings.TrimSpace(sessionID)
	state.UserID = strings.TrimSpace(state.UserID)
	state.RefreshID = strings.TrimSpace(state.RefreshID)
	if sessionID == "" || state.UserID == "" || state.RefreshID == "" || state.ExpiresAt.IsZero() {
		return errors.New("session state is incomplete")
	}
	now := time.Now().UTC()
	ttl := state.ExpiresAt.Sub(now)
	if ttl <= 0 {
		return errors.New("session already expired")
	}
	ttlMS := ceilTTLMilliseconds(ttl)
	ctx, cancel := context.WithTimeout(ctx, s.writeTimeout)
	defer cancel()
	res, err := createSessionScript.Run(
		ctx,
		s.client,
		[]string{s.sessionKey(sessionID), s.userSessionsKey(state.UserID)},
		state.UserID,
		state.RefreshID,
		state.ExpiresAt.UTC().Unix(),
		strconv.FormatBool(state.SessionOnly),
		ttlMS,
		sessionID,
		sessionIndexTTLGrace.Milliseconds(),
		now.Unix(),
		now.UnixMilli(),
	).Result()
	if err != nil {
		return authstore.ErrStoreUnavailable
	}
	created, err := scriptBoolResult(res)
	if err != nil {
		return err
	}
	if !created {
		return errors.New("session already expired")
	}
	return nil
}

// Session returns the session when still active.
// Session 返回仍然活跃的 session。
func (s *Store) Session(ctx context.Context, sessionID string) (authstore.SessionState, bool, error) {
	ctx = contextutil.Require(ctx)
	if s == nil || s.client == nil {
		return authstore.SessionState{}, false, authstore.ErrStoreUnavailable
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return authstore.SessionState{}, false, nil
	}
	ctx, cancel := context.WithTimeout(ctx, s.readTimeout)
	defer cancel()
	return s.sessionWithContext(ctx, sessionID)
}

func (s *Store) sessionWithContext(ctx context.Context, sessionID string) (authstore.SessionState, bool, error) {
	values, err := s.client.HGetAll(ctx, s.sessionKey(sessionID)).Result()
	if err != nil {
		return authstore.SessionState{}, false, authstore.ErrStoreUnavailable
	}
	if len(values) == 0 {
		return authstore.SessionState{}, false, nil
	}
	expUnix, convErr := strconv.ParseInt(values[sessionFieldExpiresAt], 10, 64)
	if convErr != nil {
		return authstore.SessionState{}, false, fmt.Errorf("invalid session expiration %q", values[sessionFieldExpiresAt])
	}
	state := authstore.SessionState{
		UserID:      values[sessionFieldUserID],
		RefreshID:   values[sessionFieldRefreshID],
		ExpiresAt:   time.Unix(expUnix, 0).UTC(),
		SessionOnly: strings.EqualFold(values[sessionFieldSessionOnly], "true"),
	}
	if state.UserID == "" || state.RefreshID == "" {
		return authstore.SessionState{}, false, nil
	}
	return state, true, nil
}

// RotateRefresh replaces the refresh state for a session.
// RotateRefresh 替换某个 session 的 refresh 状态。
func (s *Store) RotateRefresh(ctx context.Context, sessionID, userID string, expectedVersion int64, oldRefreshID, newRefreshID string, exp time.Time) (bool, error) {
	ctx = contextutil.Require(ctx)
	if s == nil || s.client == nil {
		return false, authstore.ErrStoreUnavailable
	}
	sessionID = strings.TrimSpace(sessionID)
	userID = strings.TrimSpace(userID)
	oldRefreshID = strings.TrimSpace(oldRefreshID)
	newRefreshID = strings.TrimSpace(newRefreshID)
	if sessionID == "" || userID == "" || oldRefreshID == "" || newRefreshID == "" || exp.IsZero() {
		return false, nil
	}
	now := time.Now().UTC()
	ttl := exp.Sub(now)
	if ttl <= 0 {
		return false, errors.New("session already expired")
	}
	ttlMS := ceilTTLMilliseconds(ttl)
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
		ttlMS,
		sessionID,
		sessionIndexTTLGrace.Milliseconds(),
		now.Unix(),
		now.UnixMilli(),
	).Result()
	if err != nil {
		return false, authstore.ErrStoreUnavailable
	}
	return scriptBoolResult(res)
}

// RevokeSession deletes a session.
// RevokeSession 删除 session。
func (s *Store) RevokeSession(ctx context.Context, sessionID string) error {
	ctx = contextutil.Require(ctx)
	if s == nil || s.client == nil {
		return authstore.ErrStoreUnavailable
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, s.writeTimeout)
	defer cancel()

	userID, err := s.client.HGet(ctx, s.sessionKey(sessionID), sessionFieldUserID).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return authstore.ErrStoreUnavailable
	}

	userID = strings.TrimSpace(userID)
	if userID == "" {
		if err := s.client.Del(ctx, s.sessionKey(sessionID)).Err(); err != nil {
			return authstore.ErrStoreUnavailable
		}
		return nil
	}

	now := time.Now().UTC()
	if _, err := revokeSessionScript.Run(
		ctx,
		s.client,
		[]string{s.sessionKey(sessionID), s.userSessionsKey(userID)},
		sessionID,
		sessionIndexTTLGrace.Milliseconds(),
		now.Unix(),
		now.UnixMilli(),
	).Result(); err != nil {
		return authstore.ErrStoreUnavailable
	}
	return nil
}

// Sessions returns stored, unexpired sessions for userID.
// Sessions 返回 userID 已保存且未过期的 session。
func (s *Store) Sessions(ctx context.Context, userID string) ([]authstore.SessionInfo, error) {
	ctx = contextutil.Require(ctx)
	if s == nil || s.client == nil {
		return nil, authstore.ErrStoreUnavailable
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, nil
	}

	now := time.Now().UTC()
	ctx, cancel := context.WithTimeout(ctx, s.readTimeout)
	defer cancel()

	key := s.userSessionsKey(userID)
	if err := s.trimSessionIndexWithContext(ctx, key, now); err != nil {
		return nil, authstore.ErrStoreUnavailable
	}
	ids, err := s.client.ZRangeArgs(ctx, redis.ZRangeArgs{
		Key:     key,
		Start:   "(" + strconv.FormatInt(now.Unix(), 10),
		Stop:    "+inf",
		ByScore: true,
	}).Result()
	if err != nil {
		return nil, authstore.ErrStoreUnavailable
	}

	out := make([]authstore.SessionInfo, 0, len(ids))
	stale := make([]string, 0)
	for _, sessionID := range ids {
		state, ok, err := s.sessionWithContext(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		if !ok || state.UserID != userID {
			stale = append(stale, sessionID)
			continue
		}
		out = append(out, authstore.SessionInfo{
			ID:          sessionID,
			ExpiresAt:   state.ExpiresAt,
			SessionOnly: state.SessionOnly,
		})
	}
	if len(stale) > 0 {
		if err := s.trimSessionIndexWithContext(ctx, key, now, stale...); err != nil {
			return nil, authstore.ErrStoreUnavailable
		}
	}
	return out, nil
}

// ClearUserSessions removes stored sessions for userID.
// ClearUserSessions 清理 userID 已保存的 session。
func (s *Store) ClearUserSessions(ctx context.Context, userID string) error {
	ctx = contextutil.Require(ctx)
	if s == nil || s.client == nil {
		return authstore.ErrStoreUnavailable
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, s.writeTimeout)
	defer cancel()
	if err := s.clearUserSessionsWithContext(ctx, userID); err != nil {
		return authstore.ErrStoreUnavailable
	}
	return nil
}

// ClearAllSessions removes all stored sessions.
// ClearAllSessions 清理全部已保存的 session。
func (s *Store) ClearAllSessions(ctx context.Context) error {
	ctx = contextutil.Require(ctx)
	if s == nil || s.client == nil {
		return authstore.ErrStoreUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, s.writeTimeout)
	defer cancel()
	if err := s.deleteKeysByPattern(ctx, s.sessionKey("*")); err != nil {
		return authstore.ErrStoreUnavailable
	}
	if err := s.deleteKeysByPattern(ctx, s.userSessionsKey("*")); err != nil {
		return authstore.ErrStoreUnavailable
	}
	return nil
}

func (s *Store) clearUserSessionsWithContext(ctx context.Context, userID string) error {
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

func (s *Store) deleteKeysByPattern(ctx context.Context, pattern string) error {
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

func (s *Store) userVersionKey(userID string) string {
	return s.prefix + "user:" + userID + ":version"
}

func (s *Store) userVersionWithContext(ctx context.Context, userID string) (int64, error) {
	val, err := s.client.Get(ctx, s.userVersionKey(userID)).Result()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, authstore.ErrStoreUnavailable
	}
	version, convErr := strconv.ParseInt(val, 10, 64)
	if convErr != nil {
		return 0, fmt.Errorf("invalid user version %q", val)
	}
	return version, nil
}

func (s *Store) userSessionsKey(userID string) string {
	return s.prefix + "user:" + userID + ":sessions"
}

func (s *Store) sessionKey(sessionID string) string {
	return s.prefix + "sessions:" + sessionID
}

func (s *Store) trimSessionIndexWithContext(ctx context.Context, key string, now time.Time, sessionIDs ...string) error {
	args := make([]any, 0, 3+len(sessionIDs))
	args = append(args, sessionIndexTTLGrace.Milliseconds(), now.Unix(), now.UnixMilli())
	for _, sessionID := range sessionIDs {
		args = append(args, sessionID)
	}
	_, err := trimSessionIndexScript.Run(ctx, s.client, []string{key}, args...).Result()
	return err
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

func ceilTTLMilliseconds(ttl time.Duration) int64 {
	if ttl <= 0 {
		return 0
	}
	ms := ttl / time.Millisecond
	if ttl%time.Millisecond != 0 {
		ms++
	}
	return int64(ms)
}
