package auth

import (
	"context"
	"errors"
	"strings"

	"github.com/Ithildur/EiluneKit/contextutil"
)

var (
	// ErrServiceMisconfigured reports missing Service dependencies.
	// ErrServiceMisconfigured 表示 Service 缺少依赖。
	ErrServiceMisconfigured = errors.New("auth service is misconfigured")
	// ErrTokenManagerMissing reports a missing token manager.
	// ErrTokenManagerMissing 表示缺少 token manager。
	ErrTokenManagerMissing = errors.New("token manager is required")
	// ErrLoginAuthenticatorMissing reports a missing login authenticator.
	// ErrLoginAuthenticatorMissing 表示缺少登录校验器。
	ErrLoginAuthenticatorMissing = errors.New("login authenticator is required")
	// ErrUserIDEmpty reports a successful login without a user ID.
	// ErrUserIDEmpty 表示登录成功但缺少 user ID。
	ErrUserIDEmpty = errors.New("authenticated user id is required")
)

// Service runs authentication flows without transport code.
// Service 执行与传输层无关的认证流程。
type Service struct {
	auth  TokenManager
	login LoginAuthenticator
}

// New returns a Service.
// Call New(tokenManager, loginAuthenticator).
// New 返回 Service。
// 调用 New(tokenManager, loginAuthenticator)。
func New(auth TokenManager, login LoginAuthenticator) (*Service, error) {
	if auth == nil {
		return nil, ErrTokenManagerMissing
	}
	if login == nil {
		return nil, ErrLoginAuthenticatorMissing
	}
	return &Service{
		auth:  auth,
		login: login,
	}, nil
}

// Login verifies credentials and issues tokens.
// ok reports whether the credentials were accepted.
// Login 校验凭据并签发 token。
// ok 表示凭据是否通过校验。
func (s *Service) Login(ctx context.Context, username, password string, opts IssueOptions) (Tokens, bool, error) {
	ctx = contextutil.Require(ctx)
	if err := s.requireLogin(); err != nil {
		return Tokens{}, false, err
	}

	userID, ok, err := s.login.Authenticate(ctx, username, password)
	if err != nil || !ok {
		return Tokens{}, ok, err
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return Tokens{}, false, ErrUserIDEmpty
	}

	access, accessExp, refresh, refreshExp, err := s.auth.IssueSessionTokens(ctx, userID, opts)
	if err != nil {
		return Tokens{}, false, err
	}
	return Tokens{
		UserID:           userID,
		Access:           access,
		AccessExpiresAt:  accessExp,
		Refresh:          refresh,
		RefreshExpiresAt: refreshExp,
	}, true, nil
}

// Refresh rotates a refresh token.
// ok reports whether the refresh token was accepted.
// Refresh 轮换 refresh token。
// ok 表示 refresh token 是否通过校验。
func (s *Service) Refresh(ctx context.Context, refresh string) (RefreshResult, bool, error) {
	ctx = contextutil.Require(ctx)
	if err := s.requireAuth(); err != nil {
		return RefreshResult{}, false, err
	}

	result, ok, err := s.auth.RotateRefreshTokens(ctx, refresh)
	if err != nil || !ok {
		return RefreshResult{}, ok, err
	}
	return result, true, nil
}

// Logout revokes a refresh token.
// Logout 吊销 refresh token。
func (s *Service) Logout(ctx context.Context, refresh string) error {
	ctx = contextutil.Require(ctx)
	if err := s.requireAuth(); err != nil {
		return err
	}
	return s.auth.RevokeRefresh(ctx, refresh)
}

// RevokeSession revokes one session.
// ok reports whether the session belonged to userID.
// RevokeSession 吊销一个 session。
// ok 表示该 session 是否属于 userID。
func (s *Service) RevokeSession(ctx context.Context, userID, sessionID string) (bool, error) {
	ctx = contextutil.Require(ctx)
	if err := s.requireAuth(); err != nil {
		return false, err
	}
	return s.auth.RevokeSession(ctx, userID, sessionID)
}

// RevokeAllSessions revokes all sessions for userID.
// RevokeAllSessions 吊销 userID 的全部 session。
func (s *Service) RevokeAllSessions(ctx context.Context, userID string) error {
	ctx = contextutil.Require(ctx)
	if err := s.requireAuth(); err != nil {
		return err
	}
	return s.auth.RevokeAllSessions(ctx, userID)
}

// Sessions returns stored sessions for userID.
// Sessions 返回 userID 已保存的 session。
func (s *Service) Sessions(ctx context.Context, userID string) ([]SessionInfo, error) {
	ctx = contextutil.Require(ctx)
	if err := s.requireAuth(); err != nil {
		return nil, err
	}
	lister, ok := s.auth.(SessionLister)
	if !ok {
		return nil, ErrSessionListUnsupported
	}
	return lister.Sessions(ctx, userID)
}

// ClearUserSessions revokes and removes stored sessions for userID.
// Callers must authorize userID before calling.
// ClearUserSessions 吊销并清理 userID 已保存的 session。
// 调用方必须在调用前完成 userID 授权。
func (s *Service) ClearUserSessions(ctx context.Context, userID string) error {
	ctx = contextutil.Require(ctx)
	if err := s.requireAuth(); err != nil {
		return err
	}
	cleaner, ok := s.auth.(UserSessionCleaner)
	if !ok {
		return ErrSessionClearUnsupported
	}
	return cleaner.ClearUserSessions(ctx, userID)
}

// ClearAllSessions removes all stored sessions.
// Callers must restrict this operation to trusted operators.
// ClearAllSessions 清理全部已保存的 session。
// 调用方必须限制可信操作方才能执行该操作。
func (s *Service) ClearAllSessions(ctx context.Context) error {
	ctx = contextutil.Require(ctx)
	if err := s.requireAuth(); err != nil {
		return err
	}
	cleaner, ok := s.auth.(SessionCleaner)
	if !ok {
		return ErrSessionClearUnsupported
	}
	return cleaner.ClearAllSessions(ctx)
}

func (s *Service) requireAuth() error {
	switch {
	case s == nil:
		return ErrServiceMisconfigured
	case s.auth == nil:
		return ErrTokenManagerMissing
	default:
		return nil
	}
}

func (s *Service) requireLogin() error {
	if err := s.requireAuth(); err != nil {
		return err
	}
	if s.login == nil {
		return ErrLoginAuthenticatorMissing
	}
	return nil
}
