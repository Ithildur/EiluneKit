package rbac_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	authcore "github.com/Ithildur/EiluneKit/auth"
	rbachttp "github.com/Ithildur/EiluneKit/auth/http/rbac"
	authjwt "github.com/Ithildur/EiluneKit/auth/jwt"
	corerbac "github.com/Ithildur/EiluneKit/auth/rbac"
	authstore "github.com/Ithildur/EiluneKit/auth/store"

	"github.com/go-chi/chi/v5"
)

type testUserStore struct {
	byID       map[string]corerbac.User
	byUsername map[string]string
}

func newTestUserStore(users ...corerbac.User) *testUserStore {
	store := &testUserStore{
		byID:       make(map[string]corerbac.User),
		byUsername: make(map[string]string),
	}
	for _, user := range users {
		store.byID[user.ID] = user
		store.byUsername[user.Username] = user.ID
	}
	return store
}

func (s *testUserStore) GetUser(ctx context.Context, id string) (corerbac.User, bool, error) {
	user, ok := s.byID[id]
	return user, ok, nil
}

func (s *testUserStore) GetUserByUsername(ctx context.Context, username string) (corerbac.User, bool, error) {
	id, ok := s.byUsername[username]
	if !ok {
		return corerbac.User{}, false, nil
	}
	user, ok := s.byID[id]
	return user, ok, nil
}

func newTestHandler(t *testing.T) (*rbachttp.Handler, *chi.Mux) {
	t.Helper()
	manager, err := authjwt.New("0123456789abcdef0123456789abcdef", authstore.NewMemoryStore())
	if err != nil {
		t.Fatalf("new jwt manager: %v", err)
	}
	service, err := corerbac.NewService(corerbac.ServiceOptions{
		Users: newTestUserStore(corerbac.User{
			ID:       "user-1",
			Username: "alice",
			Role:     "admin",
			Scopes:   []string{"vm:read"},
		}),
		Passwords: corerbac.PasswordVerifierFunc(func(ctx context.Context, user corerbac.User, password string) (bool, error) {
			return password == "secret", nil
		}),
		Tokens: manager,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	handler, err := rbachttp.NewHandler(service, rbachttp.Options{})
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}
	router := chi.NewRouter()
	if err := handler.Register(router); err != nil {
		t.Fatalf("register handler: %v", err)
	}
	return handler, router
}

func serve(router http.Handler, method, path, body string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if mutate != nil {
		mutate(req)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func decodePayload(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	return payload
}

func TestHandlerLoginRefreshAndMeUseJSONBearerTokens(t *testing.T) {
	_, router := newTestHandler(t)
	login := serve(router, http.MethodPost, "/auth/login", `{"username":"alice","password":"secret","persistence":"session"}`, nil)
	if login.Code != http.StatusOK {
		t.Fatalf("expected login status %d, got %d body=%s", http.StatusOK, login.Code, login.Body.String())
	}
	if cookies := login.Result().Cookies(); len(cookies) != 0 {
		t.Fatalf("expected JSON bearer login to avoid cookies, got %#v", cookies)
	}
	loginPayload := decodePayload(t, login)
	access, _ := loginPayload["access_token"].(string)
	refresh, _ := loginPayload["refresh_token"].(string)
	if access == "" || refresh == "" {
		t.Fatalf("expected access and refresh tokens, got %#v", loginPayload)
	}
	user, ok := loginPayload["user"].(map[string]any)
	if !ok || user["subject"] != "user-1" || user["role"] != "admin" || user["kind"] != string(authcore.PrincipalKindUser) {
		t.Fatalf("unexpected login user payload: %#v", loginPayload["user"])
	}

	me := serve(router, http.MethodGet, "/auth/me", "", func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer "+access)
	})
	if me.Code != http.StatusOK {
		t.Fatalf("expected me status %d, got %d body=%s", http.StatusOK, me.Code, me.Body.String())
	}

	refreshRec := serve(router, http.MethodPost, "/auth/refresh", `{"refresh_token":"`+refresh+`"}`, nil)
	if refreshRec.Code != http.StatusOK {
		t.Fatalf("expected refresh status %d, got %d body=%s", http.StatusOK, refreshRec.Code, refreshRec.Body.String())
	}
	refreshPayload := decodePayload(t, refreshRec)
	nextRefresh, _ := refreshPayload["refresh_token"].(string)
	if nextRefresh == "" || nextRefresh == refresh {
		t.Fatalf("expected rotated refresh token, got %#v", refreshPayload)
	}
}

func TestHandlerLogoutRejectsInvalidRefreshTokenAsUnauthorized(t *testing.T) {
	_, router := newTestHandler(t)

	rec := serve(router, http.MethodPost, "/auth/logout", `{"refresh_token":"not-a-jwt"}`, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusUnauthorized, rec.Code, rec.Body.String())
	}
}
