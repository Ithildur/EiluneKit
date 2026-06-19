package rbac

import (
	"context"
	"time"

	authcore "github.com/Ithildur/EiluneKit/auth"
	authjwt "github.com/Ithildur/EiluneKit/auth/jwt"
)

// User is the auth-facing user state.
// Scopes is copied before being stored or returned.
// User 是认证层需要的用户状态。
// Scopes 在存储或返回前会复制。
type User struct {
	ID       string
	Username string
	Role     string
	Scopes   []string
	Disabled bool
}

// Principal returns the authenticated principal for u.
// Principal 返回 u 对应的已认证主体。
func (u User) Principal() authcore.Principal {
	return authcore.Principal{
		Subject:  u.ID,
		Username: u.Username,
		Role:     u.Role,
		Scopes:   append([]string(nil), u.Scopes...),
		Kind:     authcore.PrincipalKindUser,
	}
}

// UserStore loads users for login and access-token validation.
// UserStore 为登录和 access token 校验加载用户。
type UserStore interface {
	GetUser(ctx context.Context, id string) (User, bool, error)
	GetUserByUsername(ctx context.Context, username string) (User, bool, error)
}

// PasswordVerifier verifies a password against a loaded user.
// PasswordVerifier 根据已加载用户校验密码。
type PasswordVerifier interface {
	VerifyPassword(ctx context.Context, user User, password string) (bool, error)
}

// PasswordVerifierFunc adapts a function to PasswordVerifier.
// PasswordVerifierFunc 将函数适配为 PasswordVerifier。
type PasswordVerifierFunc func(ctx context.Context, user User, password string) (bool, error)

// VerifyPassword calls f(ctx, user, password).
// VerifyPassword 调用 f(ctx, user, password)。
func (f PasswordVerifierFunc) VerifyPassword(ctx context.Context, user User, password string) (bool, error) {
	if f == nil {
		return false, ErrPasswordVerifierMissing
	}
	return f(ctx, user, password)
}

// TokenManager provides user session token operations.
// TokenManager 提供用户 session token 操作。
type TokenManager interface {
	ValidateAccessToken(ctx context.Context, token string) (authjwt.Claims, bool, error)
	ValidateRefreshToken(ctx context.Context, token string) (authjwt.Claims, bool, error)
	IssueSessionTokens(ctx context.Context, userID string, opts authjwt.IssueOptions) (access string, accessExp time.Time, refresh string, refreshExp time.Time, err error)
	RotateRefreshTokens(ctx context.Context, oldRefresh string) (authjwt.RefreshResult, bool, error)
	RevokeRefresh(ctx context.Context, refresh string) error
}

// IssueOptions controls token issuance behavior.
// IssueOptions 控制 token 签发行为。
type IssueOptions = authjwt.IssueOptions

// LoginRequest carries a login attempt.
// LoginRequest 保存一次登录尝试。
type LoginRequest struct {
	Username    string
	Password    string
	SessionOnly bool
	// LockoutKey identifies the login caller and must be non-empty.
	// LockoutKey 标识登录调用方，不能为空。
	LockoutKey string
}

// Tokens carries a newly issued token pair and principal.
// Tokens 保存新签发的 token 对与主体。
type Tokens struct {
	Principal        authcore.Principal
	AccessToken      string
	AccessExpiresAt  time.Time
	RefreshToken     string
	RefreshExpiresAt time.Time
	SessionOnly      bool
}

// RefreshResult carries rotated tokens and principal.
// RefreshResult 保存轮换后的 token 与主体。
type RefreshResult struct {
	Principal        authcore.Principal
	AccessToken      string
	AccessExpiresAt  time.Time
	RefreshToken     string
	RefreshExpiresAt time.Time
	SessionOnly      bool
}
