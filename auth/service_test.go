package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	authcore "github.com/Ithildur/EiluneKit/auth"
	authjwt "github.com/Ithildur/EiluneKit/auth/jwt"
)

type serviceTokenManager struct {
	revokeAllUserID string
}

func (m *serviceTokenManager) ValidateAccessToken(ctx context.Context, token string) (authjwt.Claims, bool, error) {
	return authjwt.Claims{}, false, nil
}

func (m *serviceTokenManager) IssueSessionTokens(ctx context.Context, userID string, opts authcore.IssueOptions) (string, time.Time, string, time.Time, error) {
	return "", time.Time{}, "", time.Time{}, nil
}

func (m *serviceTokenManager) RotateRefreshTokens(ctx context.Context, oldRefresh string) (authcore.RefreshResult, bool, error) {
	return authcore.RefreshResult{}, false, nil
}

func (m *serviceTokenManager) RevokeRefresh(ctx context.Context, refresh string) error {
	return nil
}

func (m *serviceTokenManager) RevokeSession(ctx context.Context, userID, sessionID string) (bool, error) {
	return false, nil
}

func (m *serviceTokenManager) RevokeAllSessions(ctx context.Context, userID string) error {
	m.revokeAllUserID = userID
	return nil
}

type serviceUserSessionCleaner struct {
	serviceTokenManager
	clearUserID string
	clearErr    error
}

func (m *serviceUserSessionCleaner) ClearUserSessions(ctx context.Context, userID string) error {
	m.clearUserID = userID
	return m.clearErr
}

func TestServiceClearUserSessionsRequiresCleaner(t *testing.T) {
	manager := &serviceTokenManager{}
	service, err := authcore.New(manager, authcore.LoginAuthenticatorFunc(func(ctx context.Context, username, password string) (string, bool, error) {
		return "", false, nil
	}))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	err = service.ClearUserSessions(context.Background(), "user-1")
	if !errors.Is(err, authcore.ErrSessionClearUnsupported) {
		t.Fatalf("expected ErrSessionClearUnsupported, got %v", err)
	}
	if manager.revokeAllUserID != "" {
		t.Fatalf("expected RevokeAllSessions not to be called, got user %q", manager.revokeAllUserID)
	}
}

func TestServiceClearUserSessionsDelegatesCleaner(t *testing.T) {
	manager := &serviceUserSessionCleaner{}
	service, err := authcore.New(manager, authcore.LoginAuthenticatorFunc(func(ctx context.Context, username, password string) (string, bool, error) {
		return "", false, nil
	}))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	if err := service.ClearUserSessions(context.Background(), "user-1"); err != nil {
		t.Fatalf("clear user sessions: %v", err)
	}
	if manager.clearUserID != "user-1" {
		t.Fatalf("expected ClearUserSessions user-1, got %q", manager.clearUserID)
	}
	if manager.revokeAllUserID != "" {
		t.Fatalf("expected RevokeAllSessions not to be called, got user %q", manager.revokeAllUserID)
	}
}
