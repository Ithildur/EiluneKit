// Package auth provides transport-neutral authentication flows.
// Package auth 提供与传输层无关的认证流程。
package auth

import (
	"context"
	"time"

	authjwt "github.com/Ithildur/EiluneKit/auth/jwt"
)

// AccessTokenValidator validates access tokens.
// AccessTokenValidator 校验 access token。
type AccessTokenValidator interface {
	ValidateAccessToken(ctx context.Context, token string) (authjwt.Claims, bool, error)
}

// TokenManager provides token and session operations for Service.
// TokenManager 为 Service 提供 token 与 session 操作。
type TokenManager interface {
	AccessTokenValidator
	IssueSessionTokens(ctx context.Context, userID string, opts IssueOptions) (access string, accessExp time.Time, refresh string, refreshExp time.Time, err error)
	RotateRefreshTokens(ctx context.Context, oldRefresh string) (RefreshResult, bool, error)
	RevokeRefresh(ctx context.Context, refresh string) error
	RevokeSession(ctx context.Context, userID, sessionID string) (bool, error)
	RevokeAllSessions(ctx context.Context, userID string) error
}

// SessionLister lists stored sessions for one user.
// SessionLister 列出单个用户已保存的 session。
type SessionLister interface {
	Sessions(ctx context.Context, userID string) ([]SessionInfo, error)
}

// UserSessionCleaner revokes and removes stored sessions for one user.
// UserSessionCleaner 吊销并清理单个用户已保存的 session。
type UserSessionCleaner interface {
	ClearUserSessions(ctx context.Context, userID string) error
}

// SessionCleaner removes all stored sessions.
// Callers must restrict this operation to trusted operators.
// SessionCleaner 清理全部已保存的 session。
// 调用方必须限制可信操作方才能执行该操作。
type SessionCleaner interface {
	ClearAllSessions(ctx context.Context) error
}

// IssueOptions controls token issuance behavior.
// IssueOptions 控制 token 签发行为。
type IssueOptions = authjwt.IssueOptions

// RefreshResult carries refreshed tokens.
// RefreshResult 保存刷新后的 token。
type RefreshResult = authjwt.RefreshResult

// SessionInfo is public session metadata for a user.
// SessionInfo 是用户可见的 session 元数据。
type SessionInfo = authjwt.SessionInfo

// ErrSessionListUnsupported reports a manager that cannot list sessions.
// ErrSessionListUnsupported 表示 manager 不支持列出 session。
var ErrSessionListUnsupported = authjwt.ErrSessionListUnsupported

// ErrSessionClearUnsupported reports a manager that cannot clear sessions.
// ErrSessionClearUnsupported 表示 manager 不支持清理 session。
var ErrSessionClearUnsupported = authjwt.ErrSessionClearUnsupported

// LoginAuthenticator verifies login credentials.
// Implementations may ignore username.
// LoginAuthenticator 校验登录凭据。
// 实现可以忽略 username。
type LoginAuthenticator interface {
	Authenticate(ctx context.Context, username, password string) (userID string, ok bool, err error)
}

// LoginAuthenticatorFunc adapts a function to LoginAuthenticator.
// LoginAuthenticatorFunc 将函数适配为 LoginAuthenticator。
type LoginAuthenticatorFunc func(ctx context.Context, username, password string) (userID string, ok bool, err error)

func (f LoginAuthenticatorFunc) Authenticate(ctx context.Context, username, password string) (userID string, ok bool, err error) {
	return f(ctx, username, password)
}

// Tokens contains access and refresh tokens.
// Tokens 保存 access 与 refresh token。
type Tokens struct {
	UserID           string
	Access           string
	AccessExpiresAt  time.Time
	Refresh          string
	RefreshExpiresAt time.Time
}
