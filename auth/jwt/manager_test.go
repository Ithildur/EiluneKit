package jwt_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	authjwt "github.com/Ithildur/EiluneKit/auth/jwt"
	authstore "github.com/Ithildur/EiluneKit/auth/store"

	"github.com/golang-jwt/jwt/v5"
)

func TestRotateRefreshTokensMemoryStoreSingleSuccess(t *testing.T) {
	mgr, err := authjwt.New("0123456789abcdef0123456789abcdef", authstore.NewMemoryStore())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	_, _, oldRefresh, _, err := mgr.IssueSessionTokens(context.Background(), "user-1", authjwt.IssueOptions{})
	if err != nil {
		t.Fatalf("issue session tokens: %v", err)
	}

	const workers = 32
	start := make(chan struct{})
	type result struct {
		refresh string
		ok      bool
		err     error
	}
	results := make(chan result, workers)
	var wg sync.WaitGroup

	for range workers {
		wg.Go(func() {
			<-start
			rotated, ok, err := mgr.RotateRefreshTokens(context.Background(), oldRefresh)
			results <- result{refresh: rotated.Refresh, ok: ok, err: err}
		})
	}

	close(start)
	wg.Wait()
	close(results)

	var successCount int
	var successfulRefresh string
	for res := range results {
		if res.err != nil {
			t.Fatalf("rotate refresh returned error: %v", res.err)
		}
		if res.ok {
			successCount++
			successfulRefresh = res.refresh
		}
	}

	if successCount != 1 {
		t.Fatalf("expected exactly 1 successful refresh rotation, got %d", successCount)
	}

	claims, ok, err := mgr.ValidateRefreshToken(context.Background(), successfulRefresh)
	if err != nil {
		t.Fatalf("validate new refresh: %v", err)
	}
	if !ok {
		t.Fatalf("expected new refresh token to be valid")
	}
	if claims.ExpiresAt == nil || claims.ExpiresAt.Time.Before(time.Now().UTC()) {
		t.Fatalf("expected new refresh token to have a future expiration")
	}
}

func TestSessionOnlySessionStateSurvivesRotation(t *testing.T) {
	store := authstore.NewMemoryStore()
	mgr, err := authjwt.New("0123456789abcdef0123456789abcdef", store)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	_, _, refresh, _, err := mgr.IssueSessionTokens(context.Background(), "user-1", authjwt.IssueOptions{
		SessionOnly: true,
	})
	if err != nil {
		t.Fatalf("issue session tokens with options: %v", err)
	}

	refreshClaims, ok, err := mgr.ValidateRefreshToken(context.Background(), refresh)
	if err != nil {
		t.Fatalf("validate refresh token: %v", err)
	}
	if !ok {
		t.Fatal("expected refresh token to be valid")
	}

	session, ok, err := store.Session(context.Background(), refreshClaims.SessionID)
	if err != nil {
		t.Fatalf("load initial session state: %v", err)
	}
	if !ok {
		t.Fatal("expected session state to exist")
	}
	if !session.SessionOnly {
		t.Fatal("expected initial session state to keep session_only")
	}

	result, ok, err := mgr.RotateRefreshTokens(context.Background(), refresh)
	if err != nil {
		t.Fatalf("rotate refresh tokens: %v", err)
	}
	if !ok {
		t.Fatal("expected refresh rotation to succeed")
	}
	if !result.SessionOnly {
		t.Fatal("expected refresh result to preserve session_only")
	}

	nextRefreshClaims, ok, err := mgr.ValidateRefreshToken(context.Background(), result.Refresh)
	if err != nil {
		t.Fatalf("validate rotated refresh token: %v", err)
	}
	if !ok {
		t.Fatal("expected rotated refresh token to be valid")
	}

	nextSession, ok, err := store.Session(context.Background(), nextRefreshClaims.SessionID)
	if err != nil {
		t.Fatalf("load rotated session state: %v", err)
	}
	if !ok {
		t.Fatal("expected rotated session state to exist")
	}
	if !nextSession.SessionOnly {
		t.Fatal("expected rotated session state to preserve session_only")
	}
}

func TestRevokeAllSessionsInvalidatesExistingTokens(t *testing.T) {
	mgr, err := authjwt.New("0123456789abcdef0123456789abcdef", authstore.NewMemoryStore())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	access, _, refresh, _, err := mgr.IssueSessionTokens(context.Background(), "user-1", authjwt.IssueOptions{})
	if err != nil {
		t.Fatalf("issue session tokens: %v", err)
	}
	if err := mgr.RevokeAllSessions(context.Background(), "user-1"); err != nil {
		t.Fatalf("revoke all sessions: %v", err)
	}

	_, ok, err := mgr.ValidateAccessToken(context.Background(), access)
	if err != nil {
		t.Fatalf("validate access token: %v", err)
	}
	if ok {
		t.Fatalf("expected access token to be invalid after revoke-all")
	}
	if _, ok, err := mgr.ValidateRefreshToken(context.Background(), refresh); err != nil || ok {
		t.Fatalf("expected refresh token to be invalid after revoke-all, ok=%v err=%v", ok, err)
	}
	sessions, err := mgr.Sessions(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("sessions after revoke-all: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected stored sessions to be cleared after revoke-all, got %#v", sessions)
	}
}

func TestManagerValidatesIssuerAndAudience(t *testing.T) {
	store := authstore.NewMemoryStore()
	const key = "0123456789abcdef0123456789abcdef"
	mgr, err := authjwt.New(key, store)
	if err != nil {
		t.Fatal(err)
	}
	access, _, refresh, _, err := mgr.IssueSessionTokens(t.Context(), "user-1", authjwt.IssueOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		opts authjwt.ManagerOptions
		want bool
	}{
		{name: "defaults", want: true},
		{name: "blank options", opts: authjwt.ManagerOptions{Issuer: " \t", Audience: " \t"}, want: true},
		{name: "different issuer", opts: authjwt.ManagerOptions{Issuer: "another-issuer"}},
		{name: "different audience", opts: authjwt.ManagerOptions{Audience: "another-client"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			validator, err := authjwt.NewWithOptions(key, store, tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			if _, ok, err := validator.ValidateAccessToken(t.Context(), access); err != nil || ok != tc.want {
				t.Fatalf("validate access: ok=%v err=%v, want ok=%v", ok, err, tc.want)
			}
			if _, ok, err := validator.ValidateRefreshToken(t.Context(), refresh); err != nil || ok != tc.want {
				t.Fatalf("validate refresh: ok=%v err=%v, want ok=%v", ok, err, tc.want)
			}
		})
	}
}

func TestManagerSessionsListsStoredSessions(t *testing.T) {
	mgr, err := authjwt.New("0123456789abcdef0123456789abcdef", authstore.NewMemoryStore())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if _, _, _, _, err := mgr.IssueSessionTokens(context.Background(), "user-1", authjwt.IssueOptions{
		SessionOnly: true,
	}); err != nil {
		t.Fatalf("issue user-1 tokens: %v", err)
	}
	if _, _, _, _, err := mgr.IssueSessionTokens(context.Background(), "user-2", authjwt.IssueOptions{}); err != nil {
		t.Fatalf("issue user-2 tokens: %v", err)
	}

	sessions, err := mgr.Sessions(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("sessions: %v", err)
	}
	if got, want := len(sessions), 1; got != want {
		t.Fatalf("expected %d session, got %d", want, got)
	}
	if sessions[0].ID == "" {
		t.Fatal("expected session id")
	}
	if sessions[0].ExpiresAt.IsZero() {
		t.Fatal("expected session expiration")
	}
	if !sessions[0].SessionOnly {
		t.Fatal("expected session_only to be preserved")
	}
}

func TestManagerClearSessions(t *testing.T) {
	store := authstore.NewMemoryStore()
	mgr, err := authjwt.New("0123456789abcdef0123456789abcdef", store)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	userAccess, _, _, _, err := mgr.IssueSessionTokens(context.Background(), "user-1", authjwt.IssueOptions{})
	if err != nil {
		t.Fatalf("issue user-1 tokens: %v", err)
	}
	otherAccess, _, _, _, err := mgr.IssueSessionTokens(context.Background(), "user-2", authjwt.IssueOptions{})
	if err != nil {
		t.Fatalf("issue user-2 tokens: %v", err)
	}

	if err := mgr.ClearUserSessions(context.Background(), "user-1"); err != nil {
		t.Fatalf("clear user sessions: %v", err)
	}
	if _, ok, err := mgr.ValidateAccessToken(context.Background(), userAccess); err != nil || ok {
		t.Fatalf("expected user-1 access token to be invalid, ok=%v err=%v", ok, err)
	}
	if _, ok, err := mgr.ValidateAccessToken(context.Background(), otherAccess); err != nil || !ok {
		t.Fatalf("expected user-2 access token to remain valid, ok=%v err=%v", ok, err)
	}

	if err := mgr.ClearAllSessions(context.Background()); err != nil {
		t.Fatalf("clear all sessions: %v", err)
	}
	if _, ok, err := mgr.ValidateAccessToken(context.Background(), otherAccess); err != nil || ok {
		t.Fatalf("expected user-2 access token to be invalid after clear all, ok=%v err=%v", ok, err)
	}
}

func TestManagerClearUserSessionsRequiresCleaner(t *testing.T) {
	store := sessionStoreOnly{SessionStore: authstore.NewMemoryStore()}
	mgr, err := authjwt.New("0123456789abcdef0123456789abcdef", store)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	access, _, _, _, err := mgr.IssueSessionTokens(context.Background(), "user-1", authjwt.IssueOptions{})
	if err != nil {
		t.Fatalf("issue tokens: %v", err)
	}

	if err := mgr.ClearUserSessions(context.Background(), "user-1"); !errors.Is(err, authjwt.ErrSessionClearUnsupported) {
		t.Fatalf("expected ErrSessionClearUnsupported, got %v", err)
	}
	if _, ok, err := mgr.ValidateAccessToken(context.Background(), access); err != nil || !ok {
		t.Fatalf("expected access token to remain valid after unsupported clear, ok=%v err=%v", ok, err)
	}

	if err := mgr.RevokeAllSessions(context.Background(), "user-1"); err != nil {
		t.Fatalf("revoke all sessions: %v", err)
	}
	if _, ok, err := mgr.ValidateAccessToken(context.Background(), access); err != nil || ok {
		t.Fatalf("expected access token to be invalid after revoke-all, ok=%v err=%v", ok, err)
	}
}

func TestManagerRejectsMisconfiguredCalls(t *testing.T) {
	var mgr *authjwt.Manager

	if _, _, _, _, err := mgr.IssueSessionTokens(context.Background(), "user-1", authjwt.IssueOptions{}); !errors.Is(err, authjwt.ErrManagerMisconfigured) {
		t.Fatalf("expected ErrManagerMisconfigured from IssueSessionTokens, got %v", err)
	}
	if _, _, err := mgr.ValidateAccessToken(context.Background(), "token"); !errors.Is(err, authjwt.ErrManagerMisconfigured) {
		t.Fatalf("expected ErrManagerMisconfigured from ValidateAccessToken, got %v", err)
	}
	if _, err := mgr.RevokeSession(context.Background(), "user-1", "sid-1"); !errors.Is(err, authjwt.ErrManagerMisconfigured) {
		t.Fatalf("expected ErrManagerMisconfigured from RevokeSession, got %v", err)
	}
	if _, err := mgr.Sessions(context.Background(), "user-1"); !errors.Is(err, authjwt.ErrManagerMisconfigured) {
		t.Fatalf("expected ErrManagerMisconfigured from Sessions, got %v", err)
	}
	if err := mgr.ClearUserSessions(context.Background(), "user-1"); !errors.Is(err, authjwt.ErrManagerMisconfigured) {
		t.Fatalf("expected ErrManagerMisconfigured from ClearUserSessions, got %v", err)
	}
	if err := mgr.ClearAllSessions(context.Background()); !errors.Is(err, authjwt.ErrManagerMisconfigured) {
		t.Fatalf("expected ErrManagerMisconfigured from ClearAllSessions, got %v", err)
	}
}

func TestManagerRejectsMissingIDs(t *testing.T) {
	mgr, err := authjwt.New("0123456789abcdef0123456789abcdef", authstore.NewMemoryStore())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	if _, _, _, _, err := mgr.IssueSessionTokens(context.Background(), "", authjwt.IssueOptions{}); !errors.Is(err, authjwt.ErrUserIDRequired) {
		t.Fatalf("expected ErrUserIDRequired from IssueSessionTokens, got %v", err)
	}
	if _, err := mgr.RevokeSession(context.Background(), "", "sid-1"); !errors.Is(err, authjwt.ErrUserIDRequired) {
		t.Fatalf("expected ErrUserIDRequired from RevokeSession, got %v", err)
	}
	if _, err := mgr.RevokeSession(context.Background(), "user-1", ""); !errors.Is(err, authjwt.ErrSessionIDRequired) {
		t.Fatalf("expected ErrSessionIDRequired from RevokeSession, got %v", err)
	}
	if err := mgr.RevokeAllSessions(context.Background(), ""); !errors.Is(err, authjwt.ErrUserIDRequired) {
		t.Fatalf("expected ErrUserIDRequired from RevokeAllSessions, got %v", err)
	}
	if _, err := mgr.Sessions(context.Background(), ""); !errors.Is(err, authjwt.ErrUserIDRequired) {
		t.Fatalf("expected ErrUserIDRequired from Sessions, got %v", err)
	}
	if err := mgr.ClearUserSessions(context.Background(), ""); !errors.Is(err, authjwt.ErrUserIDRequired) {
		t.Fatalf("expected ErrUserIDRequired from ClearUserSessions, got %v", err)
	}
}

type sessionStoreOnly struct {
	authstore.SessionStore
}

func TestManagerRejectsUserIDAliases(t *testing.T) {
	const signingKey = "0123456789abcdef0123456789abcdef"
	mgr, err := authjwt.New(signingKey, authstore.NewMemoryStore())
	if err != nil {
		t.Fatal(err)
	}
	access, _, refresh, _, err := mgr.IssueSessionTokens(t.Context(), "user-1", authjwt.IssueOptions{})
	if err != nil {
		t.Fatal(err)
	}
	claims, ok, err := mgr.ValidateAccessToken(t.Context(), access)
	if err != nil || !ok {
		t.Fatalf("validate access: ok=%v err=%v", ok, err)
	}
	for _, id := range []string{" user-1", "user-1\t", "\u00a0user-1\u00a0"} {
		t.Run(id, func(t *testing.T) {
			if _, _, _, _, err := mgr.IssueSessionTokens(t.Context(), id, authjwt.IssueOptions{}); !errors.Is(err, authjwt.ErrUserIDInvalid) {
				t.Fatalf("issue: %v", err)
			}
			if _, err := mgr.Sessions(t.Context(), id); !errors.Is(err, authjwt.ErrUserIDInvalid) {
				t.Fatalf("list: %v", err)
			}
			if _, err := mgr.RevokeSession(t.Context(), id, claims.SessionID); !errors.Is(err, authjwt.ErrUserIDInvalid) {
				t.Fatalf("revoke: %v", err)
			}
			if err := mgr.RevokeAllSessions(t.Context(), id); !errors.Is(err, authjwt.ErrUserIDInvalid) {
				t.Fatalf("revoke all: %v", err)
			}
			if err := mgr.ClearUserSessions(t.Context(), id); !errors.Is(err, authjwt.ErrUserIDInvalid) {
				t.Fatalf("clear: %v", err)
			}
		})
	}
	for _, token := range []string{access, refresh} {
		var claims authjwt.Claims
		if _, _, err := jwt.NewParser().ParseUnverified(token, &claims); err != nil {
			t.Fatal(err)
		}
		claims.Subject = " user-1 "
		alias, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(signingKey))
		if err != nil {
			t.Fatal(err)
		}
		if _, ok, err := mgr.ValidateAccessToken(t.Context(), alias); err != nil || ok {
			t.Fatalf("access validation accepted alias: ok=%v err=%v", ok, err)
		}
		if _, ok, err := mgr.ValidateRefreshToken(t.Context(), alias); err != nil || ok {
			t.Fatalf("refresh validation accepted alias: ok=%v err=%v", ok, err)
		}
		if _, ok, err := mgr.RotateRefreshTokens(t.Context(), alias); err != nil || ok {
			t.Fatalf("rotation accepted alias: ok=%v err=%v", ok, err)
		}
		if err := mgr.RevokeAccess(t.Context(), alias); !errors.Is(err, authjwt.ErrUnauthorized) {
			t.Fatalf("access revocation accepted alias: %v", err)
		}
		if err := mgr.RevokeRefresh(t.Context(), alias); !errors.Is(err, authjwt.ErrUnauthorized) {
			t.Fatalf("refresh revocation accepted alias: %v", err)
		}
	}
	if _, ok, err := mgr.ValidateAccessToken(t.Context(), access); err != nil || !ok {
		t.Fatalf("alias operations affected the canonical session: ok=%v err=%v", ok, err)
	}
	sessions, err := mgr.Sessions(t.Context(), "user-1")
	if err != nil || len(sessions) != 1 {
		t.Fatalf("alias operations changed stored sessions: count=%d err=%v", len(sessions), err)
	}
}
