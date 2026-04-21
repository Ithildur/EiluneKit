package store

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/Ithildur/EiluneKit/contextutil"
)

// MemoryStore keeps sessions in memory.
// MemoryStore 在内存中保存 session。
type MemoryStore struct {
	mu            sync.RWMutex
	sessions      map[string]memorySession
	userVersions  map[string]int64
	lastPrune     time.Time
	pruneInterval time.Duration
}

var _ SessionStore = (*MemoryStore)(nil)

type memorySession struct {
	userID      string
	refreshID   string
	exp         time.Time
	sessionOnly bool
}

// NewMemoryStore returns an in-memory SessionStore.
// NewMemoryStore 返回内存版 SessionStore。
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		sessions:      make(map[string]memorySession),
		userVersions:  make(map[string]int64),
		pruneInterval: time.Minute,
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

// CreateSession stores a session.
// CreateSession 保存 session。
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
	s.sessions[sessionID] = memorySession{
		userID:      state.UserID,
		refreshID:   state.RefreshID,
		exp:         state.ExpiresAt,
		sessionOnly: state.SessionOnly,
	}
	s.mu.Unlock()
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
	if !item.exp.After(now) {
		s.mu.Lock()
		delete(s.sessions, sessionID)
		s.mu.Unlock()
		return SessionState{}, false, nil
	}
	return SessionState{
		UserID:      item.userID,
		RefreshID:   item.refreshID,
		ExpiresAt:   item.exp,
		SessionOnly: item.sessionOnly,
	}, true, nil
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
	if !item.exp.After(now) {
		delete(s.sessions, sessionID)
		return false, nil
	}
	if item.userID != userID || item.refreshID != oldRefreshID {
		return false, nil
	}

	item.refreshID = newRefreshID
	item.exp = exp
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
	delete(s.sessions, sessionID)
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
		if !v.exp.After(now) {
			delete(s.sessions, k)
		}
	}
	s.lastPrune = now
	s.mu.Unlock()
}
