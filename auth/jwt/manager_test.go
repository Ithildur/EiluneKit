package jwt_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	authjwt "github.com/Ithildur/EiluneKit/auth/jwt"
	authstore "github.com/Ithildur/EiluneKit/auth/store"
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

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			rotated, ok, err := mgr.RotateRefreshTokens(context.Background(), oldRefresh)
			results <- result{refresh: rotated.Refresh, ok: ok, err: err}
		}()
	}

	close(start)
	wg.Wait()
	close(results)

	var successCount int32
	var successfulRefresh string
	for res := range results {
		if res.err != nil {
			t.Fatalf("rotate refresh returned error: %v", res.err)
		}
		if res.ok {
			atomic.AddInt32(&successCount, 1)
			successfulRefresh = res.refresh
		}
	}

	if got := atomic.LoadInt32(&successCount); got != 1 {
		t.Fatalf("expected exactly 1 successful refresh rotation, got %d", got)
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
}
