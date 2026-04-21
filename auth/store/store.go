// Package store defines auth session persistence used by auth/jwt.
// Package store 定义 auth/jwt 使用的认证会话持久化。
package store

import (
	"context"
	"errors"
	"time"
)

var ErrStoreUnavailable = errors.New("token store unavailable")

// SessionState stores session state for refresh rotation and revocation.
// SessionState 保存 refresh 轮换与吊销所需的 session 状态。
type SessionState struct {
	UserID      string
	RefreshID   string
	ExpiresAt   time.Time
	SessionOnly bool
}

// SessionStore persists user versions and active sessions.
// Backend availability failures must be normalized to ErrStoreUnavailable.
// SessionStore 持久化用户版本与活跃 session。
// 后端可用性错误必须统一为 ErrStoreUnavailable。
type SessionStore interface {
	UserVersion(ctx context.Context, userID string) (int64, error)
	BumpUserVersion(ctx context.Context, userID string) (int64, error)
	CreateSession(ctx context.Context, sessionID string, state SessionState) error
	Session(ctx context.Context, sessionID string) (SessionState, bool, error)
	RotateRefresh(ctx context.Context, sessionID, userID string, expectedVersion int64, oldRefreshID, newRefreshID string, exp time.Time) (bool, error)
	RevokeSession(ctx context.Context, sessionID string) error
}
