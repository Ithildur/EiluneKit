package rbac

import (
	"context"
	"time"

	authcore "github.com/Ithildur/EiluneKit/auth"
)

const (
	// LoginFailureInvalidCredentials means the username/password pair was rejected.
	// LoginFailureInvalidCredentials 表示 username/password 被拒绝。
	LoginFailureInvalidCredentials = "invalid_credentials"
	// LoginFailureDisabled means the user is disabled.
	// LoginFailureDisabled 表示用户已禁用。
	LoginFailureDisabled = "disabled"
	// LoginFailureLocked means the login key is locked.
	// LoginFailureLocked 表示登录 key 已锁定。
	LoginFailureLocked = "locked"
)

// Events configures auth lifecycle hooks.
// Hooks may be called concurrently and must not retain ctx after returning.
// Events 配置认证生命周期 hook。
// hook 可能并发调用，返回后不得继续持有 ctx。
type Events struct {
	OnLoginSuccess func(context.Context, authcore.Principal) error
	OnLoginFailure func(context.Context, LoginFailure) error
	OnTokenCreated func(context.Context, APIToken) error
	OnTokenRevoked func(context.Context, APIToken) error
}

// LoginFailure describes a rejected login attempt.
// LoginFailure 描述一次被拒绝的登录尝试。
type LoginFailure struct {
	Username string
	UserID   string
	Reason   string
	At       time.Time
}

func (e Events) loginSuccess(ctx context.Context, p authcore.Principal) error {
	if e.OnLoginSuccess == nil {
		return nil
	}
	return e.OnLoginSuccess(ctx, p)
}

func (e Events) loginFailure(ctx context.Context, event LoginFailure) error {
	if e.OnLoginFailure == nil {
		return nil
	}
	return e.OnLoginFailure(ctx, event)
}

func (e Events) tokenCreated(ctx context.Context, token APIToken) error {
	if e.OnTokenCreated == nil {
		return nil
	}
	return e.OnTokenCreated(ctx, token)
}

func (e Events) tokenRevoked(ctx context.Context, token APIToken) error {
	if e.OnTokenRevoked == nil {
		return nil
	}
	return e.OnTokenRevoked(ctx, token)
}
