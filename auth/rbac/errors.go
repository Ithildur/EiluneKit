// Package rbac provides role and scope based authentication primitives.
// Package rbac 提供基于角色与 scope 的认证基础能力。
package rbac

import (
	"errors"
	"time"
)

var (
	// ErrServiceMisconfigured reports missing Service dependencies.
	// ErrServiceMisconfigured 表示 Service 缺少依赖。
	ErrServiceMisconfigured = errors.New("rbac service is misconfigured")
	// ErrUserStoreMissing reports a missing user store.
	// ErrUserStoreMissing 表示缺少用户存储。
	ErrUserStoreMissing = errors.New("user store is required")
	// ErrPasswordVerifierMissing reports a missing password verifier.
	// ErrPasswordVerifierMissing 表示缺少密码校验器。
	ErrPasswordVerifierMissing = errors.New("password verifier is required")
	// ErrTokenManagerMissing reports a missing token manager.
	// ErrTokenManagerMissing 表示缺少 token manager。
	ErrTokenManagerMissing = errors.New("token manager is required")
	// ErrAPITokenStoreMissing reports a missing API token store.
	// ErrAPITokenStoreMissing 表示缺少 API token 存储。
	ErrAPITokenStoreMissing = errors.New("api token store is required")
	// ErrUserIDRequired reports an empty user ID.
	// ErrUserIDRequired 表示缺少 user ID。
	ErrUserIDRequired = errors.New("user id is required")
	// ErrUsernameRequired reports an empty username.
	// ErrUsernameRequired 表示缺少 username。
	ErrUsernameRequired = errors.New("username is required")
	// ErrRefreshTokenRequired reports an empty refresh token.
	// ErrRefreshTokenRequired 表示缺少 refresh token。
	ErrRefreshTokenRequired = errors.New("refresh token is required")
	// ErrBearerTokenRequired reports an empty bearer token.
	// ErrBearerTokenRequired 表示缺少 bearer token。
	ErrBearerTokenRequired = errors.New("bearer token is required")
	// ErrAPITokenIDRequired reports an empty API token ID.
	// ErrAPITokenIDRequired 表示缺少 API token ID。
	ErrAPITokenIDRequired = errors.New("api token id is required")
	// ErrLoginLocked reports a locked login key.
	// ErrLoginLocked 表示登录 key 已锁定。
	ErrLoginLocked = errors.New("login locked")
	// ErrEventFailed reports an auth hook failure.
	// ErrEventFailed 表示认证 hook 失败。
	ErrEventFailed = errors.New("auth event failed")
)

// LockedError carries the lockout expiration for ErrLoginLocked.
// LockedError 携带 ErrLoginLocked 的锁定过期时间。
type LockedError struct {
	Until time.Time
}

func (e LockedError) Error() string {
	return ErrLoginLocked.Error()
}

// Is reports whether target is ErrLoginLocked.
// Is 返回 target 是否为 ErrLoginLocked。
func (e LockedError) Is(target error) bool {
	return target == ErrLoginLocked
}
