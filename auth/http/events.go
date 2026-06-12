package authhttp

import (
	"context"
	"time"
)

// Events configures Handler event hooks.
// Hooks may be called concurrently.
// Events 配置 Handler 事件 hook。
// hook 可能并发调用。
type Events struct {
	// Login runs after credentials are accepted and tokens are issued, but before cookies and response body are written.
	// If Login returns an error, Handler attempts to revoke the new refresh session and fails the request.
	// Login 在凭据通过且 token 已签发后、cookie 和响应体写出前执行。
	// 如果 Login 返回错误，Handler 会尝试吊销新的 refresh session 并让请求失败。
	Login func(context.Context, LoginEvent) error
}

// LoginEvent describes a successful login before the response is written.
// LoginEvent 描述响应写出前的一次成功登录。
type LoginEvent struct {
	UserID           string
	Username         string
	SessionOnly      bool
	AccessExpiresAt  time.Time
	RefreshExpiresAt time.Time
}

func (e Events) handleLogin(ctx context.Context, event LoginEvent) error {
	if e.Login == nil {
		return nil
	}
	return e.Login(ctx, event)
}
