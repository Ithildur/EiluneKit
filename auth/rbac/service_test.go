package rbac_test

import (
	"context"
	"errors"
	"testing"
	"time"

	authcore "github.com/Ithildur/EiluneKit/auth"
	authjwt "github.com/Ithildur/EiluneKit/auth/jwt"
	"github.com/Ithildur/EiluneKit/auth/rbac"
	authstore "github.com/Ithildur/EiluneKit/auth/store"
)

type userStore struct {
	byID       map[string]rbac.User
	byUsername map[string]string
	getErr     error
}

func newUserStore(users ...rbac.User) *userStore {
	store := &userStore{
		byID:       make(map[string]rbac.User),
		byUsername: make(map[string]string),
	}
	for _, user := range users {
		store.byID[user.ID] = user
		store.byUsername[user.Username] = user.ID
	}
	return store
}

func (s *userStore) GetUser(ctx context.Context, id string) (rbac.User, bool, error) {
	if s.getErr != nil {
		return rbac.User{}, false, s.getErr
	}
	user, ok := s.byID[id]
	return user, ok, nil
}

func (s *userStore) GetUserByUsername(ctx context.Context, username string) (rbac.User, bool, error) {
	id, ok := s.byUsername[username]
	if !ok {
		return rbac.User{}, false, nil
	}
	user, ok := s.byID[id]
	return user, ok, nil
}

func newTestService(t *testing.T, users *userStore, opts rbac.ServiceOptions) *rbac.Service {
	t.Helper()
	if opts.Users == nil {
		opts.Users = users
	}
	if opts.Passwords == nil {
		opts.Passwords = rbac.PasswordVerifierFunc(func(ctx context.Context, user rbac.User, password string) (bool, error) {
			return password == "secret", nil
		})
	}
	if opts.Tokens == nil {
		manager, err := authjwt.New("0123456789abcdef0123456789abcdef", authstore.NewMemoryStore())
		if err != nil {
			t.Fatalf("new jwt manager: %v", err)
		}
		opts.Tokens = manager
	}
	service, err := rbac.NewService(opts)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return service
}

func loginAccessToken(t *testing.T, service *rbac.Service) string {
	t.Helper()
	tokens, ok, err := service.Login(context.Background(), rbac.LoginRequest{
		Username:   "alice",
		Password:   "secret",
		LockoutKey: "ip:127.0.0.1|username:alice",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if !ok {
		t.Fatal("expected login to succeed")
	}
	return tokens.AccessToken
}

func loginTokens(t *testing.T, service *rbac.Service) rbac.Tokens {
	t.Helper()
	tokens, ok, err := service.Login(context.Background(), rbac.LoginRequest{
		Username:   "alice",
		Password:   "secret",
		LockoutKey: "ip:127.0.0.1|username:alice",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if !ok {
		t.Fatal("expected login to succeed")
	}
	return tokens
}

func TestAuthenticateBearerLoadsCurrentUserState(t *testing.T) {
	users := newUserStore(rbac.User{ID: "user-1", Username: "alice", Role: "admin"})
	service := newTestService(t, users, rbac.ServiceOptions{})
	access := loginAccessToken(t, service)

	user := users.byID["user-1"]
	user.Disabled = true
	users.byID["user-1"] = user

	_, ok, err := service.AuthenticateBearer(context.Background(), access)
	if err != nil {
		t.Fatalf("authenticate bearer: %v", err)
	}
	if ok {
		t.Fatal("expected disabled user token to be rejected")
	}
}

func TestAuthenticateBearerRejectsBumpedSessionVersion(t *testing.T) {
	users := newUserStore(rbac.User{ID: "user-1", Username: "alice", Role: "admin"})
	store := authstore.NewMemoryStore()
	manager, err := authjwt.New("0123456789abcdef0123456789abcdef", store)
	if err != nil {
		t.Fatalf("new jwt manager: %v", err)
	}
	service := newTestService(t, users, rbac.ServiceOptions{Tokens: manager})
	access := loginAccessToken(t, service)

	if _, err := store.BumpUserVersion(context.Background(), "user-1"); err != nil {
		t.Fatalf("bump user version: %v", err)
	}

	_, ok, err := service.AuthenticateBearer(context.Background(), access)
	if err != nil {
		t.Fatalf("authenticate bearer: %v", err)
	}
	if ok {
		t.Fatal("expected stale user-version token to be rejected")
	}
}

func TestRefreshDoesNotConsumeTokenWhenPrincipalLoadFails(t *testing.T) {
	users := newUserStore(rbac.User{ID: "user-1", Username: "alice", Role: "admin"})
	service := newTestService(t, users, rbac.ServiceOptions{})
	tokens := loginTokens(t, service)

	users.getErr = errors.New("store unavailable")
	if _, _, err := service.Refresh(context.Background(), tokens.RefreshToken); err == nil {
		t.Fatal("expected refresh to fail while loading current user")
	}

	users.getErr = nil
	if _, ok, err := service.Refresh(context.Background(), tokens.RefreshToken); err != nil || !ok {
		t.Fatalf("expected original refresh token to remain usable, ok=%v err=%v", ok, err)
	}
}

func TestLoginLockoutClearedAfterSuccess(t *testing.T) {
	users := newUserStore(rbac.User{ID: "user-1", Username: "alice", Role: "admin"})
	lockout := rbac.NewMemoryLockout(rbac.MemoryLockoutOptions{
		MaxFailures: 2,
		Window:      time.Minute,
		Lockout:     time.Minute,
	})
	service := newTestService(t, users, rbac.ServiceOptions{Lockout: lockout})
	ctx := context.Background()
	req := rbac.LoginRequest{Username: "alice", LockoutKey: "ip:127.0.0.1|username:alice"}

	req.Password = "wrong"
	if _, ok, err := service.Login(ctx, req); err != nil || ok {
		t.Fatalf("expected first bad password to be rejected without error, ok=%v err=%v", ok, err)
	}
	req.Password = "secret"
	if _, ok, err := service.Login(ctx, req); err != nil || !ok {
		t.Fatalf("expected successful login to clear lockout state, ok=%v err=%v", ok, err)
	}
	req.Password = "wrong"
	if _, ok, err := service.Login(ctx, req); err != nil || ok {
		t.Fatalf("expected bad password after success to be first fresh failure, ok=%v err=%v", ok, err)
	}
	req.Password = "secret"
	if _, ok, err := service.Login(ctx, req); err != nil || !ok {
		t.Fatalf("expected login to remain unlocked after one fresh failure, ok=%v err=%v", ok, err)
	}
}

func TestLoginLockoutBlocksAfterFailures(t *testing.T) {
	users := newUserStore(rbac.User{ID: "user-1", Username: "alice", Role: "admin"})
	service := newTestService(t, users, rbac.ServiceOptions{
		Lockout: rbac.NewMemoryLockout(rbac.MemoryLockoutOptions{
			MaxFailures: 2,
			Window:      time.Minute,
			Lockout:     time.Minute,
		}),
	})
	req := rbac.LoginRequest{Username: "alice", Password: "wrong", LockoutKey: "ip:127.0.0.1|username:alice"}

	if _, ok, err := service.Login(context.Background(), req); err != nil || ok {
		t.Fatalf("expected first bad password to be rejected without error, ok=%v err=%v", ok, err)
	}
	if _, _, err := service.Login(context.Background(), req); !errors.Is(err, rbac.ErrLoginLocked) {
		t.Fatalf("expected ErrLoginLocked on threshold failure, got %v", err)
	}
	req.Password = "secret"
	if _, _, err := service.Login(context.Background(), req); !errors.Is(err, rbac.ErrLoginLocked) {
		t.Fatalf("expected ErrLoginLocked, got %v", err)
	}
}

func TestLoginUsesDefaultLockout(t *testing.T) {
	users := newUserStore(rbac.User{ID: "user-1", Username: "alice", Role: "admin"})
	service := newTestService(t, users, rbac.ServiceOptions{})
	req := rbac.LoginRequest{Username: "alice", Password: "wrong", LockoutKey: "ip:127.0.0.1|username:alice"}

	for range 4 {
		if _, ok, err := service.Login(context.Background(), req); err != nil || ok {
			t.Fatalf("expected bad password to be rejected without lockout, ok=%v err=%v", ok, err)
		}
	}
	if _, _, err := service.Login(context.Background(), req); !errors.Is(err, rbac.ErrLoginLocked) {
		t.Fatalf("expected default lockout to block on threshold, got %v", err)
	}
}

type apiTokenStore struct {
	created rbac.APIToken
	token   rbac.APIToken
	hash    string
	deleted string
	used    string
	usedAt  time.Time
}

func (s *apiTokenStore) GetAPITokenByHash(ctx context.Context, hash string) (rbac.APIToken, bool, error) {
	s.hash = hash
	if s.token.ID == "" {
		return rbac.APIToken{}, false, nil
	}
	return s.token, true, nil
}

func (s *apiTokenStore) MarkAPITokenUsed(ctx context.Context, id string, at time.Time) error {
	s.used = id
	s.usedAt = at
	return nil
}

func (s *apiTokenStore) CreateAPIToken(ctx context.Context, token rbac.APIToken) error {
	s.created = token
	s.token = token
	return nil
}

func (s *apiTokenStore) DeleteAPIToken(ctx context.Context, id string) (rbac.APIToken, bool, error) {
	s.deleted = id
	if s.token.ID != id {
		return rbac.APIToken{}, false, nil
	}
	token := s.token
	s.token = rbac.APIToken{}
	return token, true, nil
}

func TestCreateAndValidateAPITokenUsesHashOnly(t *testing.T) {
	users := newUserStore(rbac.User{ID: "user-1", Username: "alice", Role: "admin"})
	apiTokens := &apiTokenStore{}
	service := newTestService(t, users, rbac.ServiceOptions{APITokens: apiTokens})

	created, err := service.CreateAPIToken(context.Background(), rbac.CreateAPITokenRequest{
		CreatedBy: "user-1",
		Role:      "operator",
		Scopes:    []string{"vm:read"},
	})
	if err != nil {
		t.Fatalf("create api token: %v", err)
	}
	if created.Raw == "" {
		t.Fatal("expected raw token to be returned once")
	}
	if created.Token.Hash != "" {
		t.Fatalf("expected returned token metadata to hide hash, got %#v", created.Token)
	}
	if apiTokens.created.Hash == "" || apiTokens.created.Hash == created.Raw {
		t.Fatalf("expected stored hash, got %#v", apiTokens.created)
	}
	if apiTokens.created.Hash != rbac.HashToken(created.Raw) {
		t.Fatalf("expected stored hash to match raw token")
	}
	if apiTokens.created.Prefix == "" || apiTokens.created.Prefix != created.Raw[:len(apiTokens.created.Prefix)] {
		t.Fatalf("expected stored prefix to be derived from raw token")
	}

	principal, ok, err := service.ValidateAPIToken(context.Background(), created.Raw)
	if err != nil {
		t.Fatalf("validate api token: %v", err)
	}
	if !ok {
		t.Fatal("expected api token to be accepted")
	}
	if principal.Kind != authcore.PrincipalKindAPIToken {
		t.Fatalf("expected api token principal kind, got %q", principal.Kind)
	}
	if principal.Subject != apiTokens.created.ID || principal.Role != "operator" || !principal.HasScope("vm:read") {
		t.Fatalf("unexpected principal: %#v", principal)
	}
	if apiTokens.hash != rbac.HashToken(created.Raw) {
		t.Fatalf("expected lookup by hash, got %q", apiTokens.hash)
	}
	if apiTokens.used != apiTokens.created.ID || apiTokens.usedAt.IsZero() {
		t.Fatalf("expected accepted api token to be marked used, used=%q at=%v", apiTokens.used, apiTokens.usedAt)
	}
}

func TestValidateAPITokenDoesNotMarkRejectedTokenUsed(t *testing.T) {
	users := newUserStore(rbac.User{ID: "user-1", Username: "alice", Role: "admin"})
	raw := "ekt_disabled"
	apiTokens := &apiTokenStore{
		token: rbac.APIToken{
			ID:       "token-1",
			Hash:     rbac.HashToken(raw),
			Disabled: true,
		},
	}
	service := newTestService(t, users, rbac.ServiceOptions{APITokens: apiTokens})

	_, ok, err := service.ValidateAPIToken(context.Background(), raw)
	if err != nil {
		t.Fatalf("validate api token: %v", err)
	}
	if ok {
		t.Fatal("expected disabled api token to be rejected")
	}
	if apiTokens.used != "" {
		t.Fatalf("expected rejected api token not to be marked used, got %q", apiTokens.used)
	}
}

func TestCreateAPITokenDeletesTokenWhenCreatedHookFails(t *testing.T) {
	users := newUserStore(rbac.User{ID: "user-1", Username: "alice", Role: "admin"})
	apiTokens := &apiTokenStore{}
	service := newTestService(t, users, rbac.ServiceOptions{
		APITokens: apiTokens,
		Events: rbac.Events{
			OnTokenCreated: func(context.Context, rbac.APIToken) error {
				return errors.New("audit unavailable")
			},
		},
	})

	if _, err := service.CreateAPIToken(context.Background(), rbac.CreateAPITokenRequest{CreatedBy: "user-1"}); !errors.Is(err, rbac.ErrEventFailed) {
		t.Fatalf("expected ErrEventFailed, got %v", err)
	}
	if apiTokens.deleted == "" || apiTokens.deleted != apiTokens.created.ID {
		t.Fatalf("expected failed token to be deleted, created=%q deleted=%q", apiTokens.created.ID, apiTokens.deleted)
	}
	if apiTokens.token.ID != "" {
		t.Fatalf("expected store token to be cleared, got %#v", apiTokens.token)
	}
}

func TestCreateAPITokenHookReceivesPublicMetadata(t *testing.T) {
	users := newUserStore(rbac.User{ID: "user-1", Username: "alice", Role: "admin"})
	apiTokens := &apiTokenStore{}
	var eventToken rbac.APIToken
	service := newTestService(t, users, rbac.ServiceOptions{
		APITokens: apiTokens,
		Events: rbac.Events{
			OnTokenCreated: func(ctx context.Context, token rbac.APIToken) error {
				eventToken = token
				return nil
			},
		},
	})

	created, err := service.CreateAPIToken(context.Background(), rbac.CreateAPITokenRequest{CreatedBy: "user-1"})
	if err != nil {
		t.Fatalf("create api token: %v", err)
	}
	if eventToken.ID != created.Token.ID || eventToken.Prefix == "" {
		t.Fatalf("unexpected event token: %#v", eventToken)
	}
	if eventToken.Hash != "" {
		t.Fatalf("expected event token to hide hash, got %#v", eventToken)
	}
}

func TestDeleteAPITokenReportsDeletedWhenRevokedHookFails(t *testing.T) {
	users := newUserStore(rbac.User{ID: "user-1", Username: "alice", Role: "admin"})
	apiTokens := &apiTokenStore{}
	service := newTestService(t, users, rbac.ServiceOptions{
		APITokens: apiTokens,
		Events: rbac.Events{
			OnTokenRevoked: func(context.Context, rbac.APIToken) error {
				return errors.New("audit unavailable")
			},
		},
	})

	created, err := service.CreateAPIToken(context.Background(), rbac.CreateAPITokenRequest{CreatedBy: "user-1"})
	if err != nil {
		t.Fatalf("create api token: %v", err)
	}
	ok, err := service.DeleteAPIToken(context.Background(), created.Token.ID)
	if !ok {
		t.Fatal("expected token deletion to be reported")
	}
	if !errors.Is(err, rbac.ErrEventFailed) {
		t.Fatalf("expected ErrEventFailed, got %v", err)
	}
}
