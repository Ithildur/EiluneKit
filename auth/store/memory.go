package store

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Ithildur/EiluneKit/contextutil"
)

// MemoryStore keeps sessions in memory.
// MemoryStore 在内存中保存 session。
type MemoryStore struct {
	mu                 sync.RWMutex
	sessions           map[string]SessionState
	userSessions       map[string]map[string]struct{}
	userVersions       map[string]int64
	maxSessionsPerUser int
	lastPrune          time.Time
	pruneInterval      time.Duration
}

var (
	_ SessionStore       = (*MemoryStore)(nil)
	_ SessionLister      = (*MemoryStore)(nil)
	_ UserSessionCleaner = (*MemoryStore)(nil)
	_ SessionCleaner     = (*MemoryStore)(nil)
)

// NewMemoryStore returns an in-memory SessionStore with a limit of 255 sessions per user.
// NewMemoryStore 返回内存版 SessionStore，每个用户最多保存 255 个会话。
func NewMemoryStore() *MemoryStore {
	return NewMemoryStoreWithLimit(DefaultMaxSessionsPerUser)
}

// NewMemoryStoreWithLimit sets the maximum number of unexpired sessions per user.
// Zero disables the limit; negative values panic.
// NewMemoryStoreWithLimit 设置每个用户未过期会话的数量上限。
// 零值禁用上限；负数会 panic。
func NewMemoryStoreWithLimit(limit int) *MemoryStore {
	if limit < 0 {
		panic("store: session limit must not be negative")
	}
	return &MemoryStore{
		sessions:           make(map[string]SessionState),
		userSessions:       make(map[string]map[string]struct{}),
		userVersions:       make(map[string]int64),
		maxSessionsPerUser: limit,
		pruneInterval:      time.Minute,
	}
}

// UserVersion returns the current version for userID.
// UserVersion 返回 userID 的当前版本。
func (s *MemoryStore) UserVersion(ctx context.Context, userID string) (int64, error) {
	contextutil.Require(ctx)
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return 0, nil
	}
	s.mu.RLock()
	version := s.userVersions[userID]
	s.mu.RUnlock()
	return version, nil
}

// BumpUserVersion invalidates all sessions for userID.
// BumpUserVersion 通过提升版本使 userID 的全部 session 失效。
func (s *MemoryStore) BumpUserVersion(ctx context.Context, userID string) (int64, error) {
	contextutil.Require(ctx)
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return 0, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.userVersions[userID] + 1
	s.userVersions[userID] = next
	return next, nil
}

// CreateSession stores a session, returning ErrSessionLimitReached when the user is full.
// Replacing an existing session for the same user does not consume another slot.
// CreateSession 保存 session；用户会话已满时返回 ErrSessionLimitReached。
// 替换同一用户已有的 session 不占用额外名额。
func (s *MemoryStore) CreateSession(ctx context.Context, sessionID string, state SessionState) error {
	contextutil.Require(ctx)
	if s == nil {
		return ErrStoreUnavailable
	}
	sessionID = strings.TrimSpace(sessionID)
	state.UserID = strings.TrimSpace(state.UserID)
	state.RefreshID = strings.TrimSpace(state.RefreshID)
	if sessionID == "" || state.UserID == "" || state.RefreshID == "" || state.ExpiresAt.IsZero() {
		return errors.New("session state is incomplete")
	}
	now := time.Now().UTC()
	if !state.ExpiresAt.After(now) {
		return errors.New("session already expired")
	}
	s.pruneExpired(now, false)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.maxSessionsPerUser > 0 && len(s.userSessions[state.UserID]) >= s.maxSessionsPerUser {
		for id := range s.userSessions[state.UserID] {
			if !s.sessions[id].ExpiresAt.After(now) {
				s.deleteSession(id)
			}
		}
		_, replacing := s.userSessions[state.UserID][sessionID]
		if !replacing && len(s.userSessions[state.UserID]) >= s.maxSessionsPerUser {
			return ErrSessionLimitReached
		}
	}
	s.deleteSession(sessionID)
	if s.userSessions[state.UserID] == nil {
		s.userSessions[state.UserID] = make(map[string]struct{})
	}
	s.sessions[sessionID] = state
	s.userSessions[state.UserID][sessionID] = struct{}{}
	return nil
}

// Session returns the session when still active.
// Session 返回仍然活跃的 session。
func (s *MemoryStore) Session(ctx context.Context, sessionID string) (SessionState, bool, error) {
	contextutil.Require(ctx)
	if s == nil {
		return SessionState{}, false, ErrStoreUnavailable
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return SessionState{}, false, nil
	}
	now := time.Now().UTC()

	s.mu.RLock()
	item, ok := s.sessions[sessionID]
	s.mu.RUnlock()
	if !ok {
		return SessionState{}, false, nil
	}
	if !item.ExpiresAt.After(now) {
		s.mu.Lock()
		if current, ok := s.sessions[sessionID]; ok && !current.ExpiresAt.After(now) {
			s.deleteSession(sessionID)
		}
		s.mu.Unlock()
		return SessionState{}, false, nil
	}
	return item, true, nil
}

// RotateRefresh replaces the refresh state for a session.
// RotateRefresh 替换某个 session 的 refresh 状态。
func (s *MemoryStore) RotateRefresh(ctx context.Context, sessionID, userID string, expectedVersion int64, oldRefreshID, newRefreshID string, exp time.Time) (bool, error) {
	contextutil.Require(ctx)
	if s == nil {
		return false, ErrStoreUnavailable
	}
	sessionID = strings.TrimSpace(sessionID)
	userID = strings.TrimSpace(userID)
	oldRefreshID = strings.TrimSpace(oldRefreshID)
	newRefreshID = strings.TrimSpace(newRefreshID)
	if sessionID == "" || userID == "" || oldRefreshID == "" || newRefreshID == "" || exp.IsZero() {
		return false, nil
	}
	now := time.Now().UTC()
	if !exp.After(now) {
		return false, errors.New("session already expired")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.userVersions[userID] != expectedVersion {
		return false, nil
	}

	item, ok := s.sessions[sessionID]
	if !ok {
		return false, nil
	}
	if !item.ExpiresAt.After(now) {
		s.deleteSession(sessionID)
		return false, nil
	}
	if item.UserID != userID || item.RefreshID != oldRefreshID {
		return false, nil
	}

	item.RefreshID = newRefreshID
	item.ExpiresAt = exp
	s.sessions[sessionID] = item
	return true, nil
}

// RevokeSession deletes a session.
// RevokeSession 删除 session。
func (s *MemoryStore) RevokeSession(ctx context.Context, sessionID string) error {
	contextutil.Require(ctx)
	if s == nil {
		return ErrStoreUnavailable
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	s.mu.Lock()
	s.deleteSession(sessionID)
	s.mu.Unlock()
	return nil
}

// Sessions returns stored, unexpired sessions for userID.
// Sessions 返回 userID 已保存且未过期的 session。
func (s *MemoryStore) Sessions(ctx context.Context, userID string) ([]SessionInfo, error) {
	contextutil.Require(ctx)
	if s == nil {
		return nil, ErrStoreUnavailable
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, nil
	}
	now := time.Now().UTC()
	s.pruneExpired(now, false)

	s.mu.RLock()
	out := make([]SessionInfo, 0)
	for sessionID := range s.userSessions[userID] {
		item := s.sessions[sessionID]
		if !item.ExpiresAt.After(now) {
			continue
		}
		out = append(out, SessionInfo{
			ID:          sessionID,
			ExpiresAt:   item.ExpiresAt,
			SessionOnly: item.SessionOnly,
		})
	}
	s.mu.RUnlock()

	sort.Slice(out, func(i, j int) bool {
		if out[i].ExpiresAt.Equal(out[j].ExpiresAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].ExpiresAt.Before(out[j].ExpiresAt)
	})
	return out, nil
}

// ClearUserSessions removes stored sessions for userID.
// ClearUserSessions 清理 userID 已保存的 session。
func (s *MemoryStore) ClearUserSessions(ctx context.Context, userID string) error {
	contextutil.Require(ctx)
	if s == nil {
		return ErrStoreUnavailable
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil
	}
	s.mu.Lock()
	for sessionID := range s.userSessions[userID] {
		s.deleteSession(sessionID)
	}
	s.mu.Unlock()
	return nil
}

// ClearAllSessions removes all stored sessions.
// ClearAllSessions 清理全部已保存的 session。
func (s *MemoryStore) ClearAllSessions(ctx context.Context) error {
	contextutil.Require(ctx)
	if s == nil {
		return ErrStoreUnavailable
	}
	s.mu.Lock()
	s.sessions = make(map[string]SessionState)
	s.userSessions = make(map[string]map[string]struct{})
	s.lastPrune = time.Time{}
	s.mu.Unlock()
	return nil
}

// Prune removes expired sessions.
// Prune 清理过期 session。
func (s *MemoryStore) Prune() {
	if s == nil {
		return
	}
	s.pruneExpired(time.Now().UTC(), true)
}

func (s *MemoryStore) pruneExpired(now time.Time, force bool) {
	s.mu.Lock()
	if !force && s.pruneInterval > 0 && !s.lastPrune.IsZero() && now.Sub(s.lastPrune) < s.pruneInterval {
		s.mu.Unlock()
		return
	}
	for k, v := range s.sessions {
		if !v.ExpiresAt.After(now) {
			s.deleteSession(k)
		}
	}
	s.lastPrune = now
	s.mu.Unlock()
}

// deleteSession updates both indexes while the caller holds s.mu for writing.
// deleteSession 在调用方持有 s.mu 写锁时同步更新两个索引。
func (s *MemoryStore) deleteSession(sessionID string) {
	state, ok := s.sessions[sessionID]
	if !ok {
		return
	}
	delete(s.sessions, sessionID)
	ids := s.userSessions[state.UserID]
	delete(ids, sessionID)
	if len(ids) == 0 {
		delete(s.userSessions, state.UserID)
	}
}
