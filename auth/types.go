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

// IssueOptions controls token issuance behavior.
// IssueOptions 控制 token 签发行为。
type IssueOptions = authjwt.IssueOptions

// RefreshResult carries refreshed tokens.
// RefreshResult 保存刷新后的 token。
type RefreshResult = authjwt.RefreshResult

// LoginAuthenticator verifies login credentials.
// Implementations may ignore username.
// LoginAuthenticator 校验登录凭据。
// 实现可以忽略 username。
type LoginAuthenticator interface {
	AuthenticateUserPassword(ctx context.Context, username, password string) (userID string, ok bool, err error)
}

// LoginAuthenticatorFunc adapts a function to LoginAuthenticator.
// LoginAuthenticatorFunc 将函数适配为 LoginAuthenticator。
type LoginAuthenticatorFunc func(ctx context.Context, username, password string) (userID string, ok bool, err error)

func (f LoginAuthenticatorFunc) AuthenticateUserPassword(ctx context.Context, username, password string) (userID string, ok bool, err error) {
	return f(ctx, username, password)
}

// Tokens contains access and refresh tokens.
// Tokens 保存 access 与 refresh token。
type Tokens struct {
	Access           string
	AccessExpiresAt  time.Time
	Refresh          string
	RefreshExpiresAt time.Time
}
