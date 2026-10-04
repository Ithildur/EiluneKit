package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	authcore "github.com/Ithildur/EiluneKit/auth"
	authjwt "github.com/Ithildur/EiluneKit/auth/jwt"
	authstore "github.com/Ithildur/EiluneKit/auth/store"
)

func TestLoginRejectsNoncanonicalUserID(t *testing.T) {
	manager, err := authjwt.New("0123456789abcdef0123456789abcdef", authstore.NewMemoryStore())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{" user-1", "user-1\t", "\u00a0user-1\u00a0"} {
		t.Run(id, func(t *testing.T) {
			if _, err := authcore.NewStaticPassword(id, "secret"); !errors.Is(err, authcore.ErrUserIDInvalid) {
				t.Fatalf("static password accepted noncanonical ID: %v", err)
			}
			service, err := authcore.New(manager, authcore.LoginAuthenticatorFunc(func(context.Context, string, string) (string, bool, error) {
				return id, true, nil
			}))
			if err != nil {
				t.Fatal(err)
			}
			tokens, ok, err := service.Login(t.Context(), "alice", "secret", authcore.IssueOptions{})
			if !errors.Is(err, authcore.ErrUserIDInvalid) || ok || tokens.Access != "" || tokens.Refresh != "" {
				t.Fatalf("login accepted noncanonical ID: ok=%v err=%v", ok, err)
			}
		})
	}
	sessions, err := manager.Sessions(t.Context(), "user-1")
	if err != nil || len(sessions) != 0 {
		t.Fatalf("rejected logins created sessions for another ID: count=%d err=%v", len(sessions), err)
	}
}

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
