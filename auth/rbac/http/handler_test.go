package rbachttp_test

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"reflect"
	"strings"
	"testing"
	"time"

	authcore "github.com/Ithildur/EiluneKit/auth"
	authjwt "github.com/Ithildur/EiluneKit/auth/jwt"
	corerbac "github.com/Ithildur/EiluneKit/auth/rbac"
	rbachttp "github.com/Ithildur/EiluneKit/auth/rbac/http"
	authstore "github.com/Ithildur/EiluneKit/auth/store"
	"github.com/Ithildur/EiluneKit/tools/openapi"

	"github.com/go-chi/chi/v5"
)

type testUserStore struct {
	byID       map[string]corerbac.User
	byUsername map[string]string
}

type captureLockout struct {
	keys []string
}

type countedLockout struct {
	corerbac.Lockout
	checks int
}

func (l *countedLockout) Check(ctx context.Context, key string) (time.Time, bool, error) {
	l.checks++
	return l.Lockout.Check(ctx, key)
}

func (l *captureLockout) Check(ctx context.Context, key string) (time.Time, bool, error) {
	return time.Time{}, false, nil
}

func (l *captureLockout) RecordFailure(ctx context.Context, key string) (time.Time, bool, error) {
	l.keys = append(l.keys, key)
	return time.Time{}, false, nil
}

func (l *captureLockout) Clear(ctx context.Context, key string) error {
	return nil
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

func newTestHandler(t *testing.T, options ...rbachttp.Options) (*rbachttp.Handler, *chi.Mux) {
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
	var opts rbachttp.Options
	if len(options) > 0 {
		opts = options[0]
	}
	handler, err := rbachttp.NewHandler(service, opts)
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}
	router := chi.NewRouter()
	if err := handler.Register(router); err != nil {
		t.Fatalf("register handler: %v", err)
	}
	return handler, router
}

func TestEmptyPasswordPreservesLockoutAndShortCircuit(t *testing.T) {
	now := time.Now()
	until := now.Add(time.Hour)
	lockout := &countedLockout{Lockout: corerbac.NewMemoryLockout(corerbac.MemoryLockoutOptions{
		MaxFailures: 2, Window: time.Minute, Lockout: time.Hour,
		Now: func() time.Time { return now },
	})}
	manager, err := authjwt.New("0123456789abcdef0123456789abcdef", authstore.NewMemoryStore())
	if err != nil {
		t.Fatal(err)
	}
	verifications := 0
	service, err := corerbac.NewService(corerbac.ServiceOptions{
		Users: newTestUserStore(corerbac.User{ID: "user-1", Username: "alice"}),
		Passwords: corerbac.PasswordVerifierFunc(func(context.Context, corerbac.User, string) (bool, error) {
			verifications++
			return false, nil
		}),
		Tokens: manager, Lockout: lockout,
	})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := rbachttp.NewHandler(service, rbachttp.Options{RateLimit: &rbachttp.RateLimitOptions{Disabled: true}})
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	if err := handler.Register(router); err != nil {
		t.Fatal(err)
	}
	wrong := `{"username":"alice","password":"wrong"}`
	if w := serve(router, http.MethodPost, "/auth/login", wrong, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("first failure: %d %s", w.Code, w.Body.String())
	}
	if w := serve(router, http.MethodPost, "/auth/login", wrong, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("lock: %d %s", w.Code, w.Body.String())
	}
	now = now.Add(2 * time.Minute)
	checks := lockout.checks
	for range 2 {
		if w := serve(router, http.MethodPost, "/auth/login", `{"username":"alice","password":""}`, nil); w.Code != http.StatusUnauthorized {
			t.Fatalf("empty password unlocked the caller: %d %s", w.Code, w.Body.String())
		}
	}
	if lockout.checks != checks || verifications != 2 {
		t.Fatal("empty password did not short-circuit")
	}
	if w := serve(router, http.MethodPost, "/auth/login", wrong, nil); w.Code != http.StatusUnauthorized || verifications != 2 {
		t.Fatalf("locked request reached verification: %d checks=%d", w.Code, verifications)
	}
	now = until
	if w := serve(router, http.MethodPost, "/auth/login", `{"username":"alice","password":""}`, nil); w.Code != http.StatusUnauthorized || verifications != 2 {
		t.Fatalf("expired lock did not restart counting: %d checks=%d", w.Code, verifications)
	}
}

func TestHandlerBasePath(t *testing.T) {
	for _, path := range []string{"auth", "/auth/", "/", "//auth", " /auth", "/auth "} {
		if _, err := rbachttp.NewHandler(&corerbac.Service{}, rbachttp.Options{BasePath: new(path)}); err == nil {
			t.Fatalf("accepted invalid BasePath %q", path)
		}
	}
	basePath := ""
	h, router := newTestHandler(t, rbachttp.Options{BasePath: &basePath})
	basePath = "changed"
	if got := h.Routes()[0].Path; got != "/login" {
		t.Fatalf("BasePath changed after construction: %q", got)
	}
	rec := serve(router, http.MethodGet, "/me", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("root endpoint status: %d", rec.Code)
	}
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

func TestRoutesGenerateOpenAPI(t *testing.T) {
	handler, _ := newTestHandler(t)
	if _, err := openapi.Generate(handler.Routes(), openapi.Options{Title: "RBAC API", Version: "1"}); err != nil {
		t.Fatalf("generate OpenAPI: %v", err)
	}
}

func TestHandlerLoginRejectsMissingClientIP(t *testing.T) {
	_, router := newTestHandler(t)

	rec := serve(router, http.MethodPost, "/auth/login", `{"username":"alice","password":"secret","persistence":"session"}`, func(req *http.Request) {
		req.RemoteAddr = ""
	})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusInternalServerError, rec.Code, rec.Body.String())
	}
	payload := decodePayload(t, rec)
	if payload["code"] != "auth_misconfigured" || payload["message"] != "auth is misconfigured" {
		t.Fatalf("unexpected error payload: %#v", payload)
	}
}

func TestHandlerLoginUsesBoundedLockoutKey(t *testing.T) {
	manager, err := authjwt.New("0123456789abcdef0123456789abcdef", authstore.NewMemoryStore())
	if err != nil {
		t.Fatalf("new jwt manager: %v", err)
	}
	lockout := &captureLockout{}
	service, err := corerbac.NewService(corerbac.ServiceOptions{
		Users:     newTestUserStore(),
		Passwords: corerbac.PasswordVerifierFunc(func(ctx context.Context, user corerbac.User, password string) (bool, error) { return false, nil }),
		Tokens:    manager,
		Lockout:   lockout,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	headers := []string{"X-Real-IP"}
	handler, err := rbachttp.NewHandler(service, rbachttp.Options{
		TrustedProxies:  []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")},
		ClientIPHeaders: headers,
	})
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}
	router := chi.NewRouter()
	if err := handler.Register(router); err != nil {
		t.Fatalf("register handler: %v", err)
	}
	headers[0] = "X-Forwarded-For"

	username := strings.Repeat("a", 900*1024)
	body, err := json.Marshal(map[string]string{
		"username": username,
		"password": "wrong",
	})
	if err != nil {
		t.Fatalf("marshal login body: %v", err)
	}
	rec := serve(router, http.MethodPost, "/auth/login", string(body), func(req *http.Request) {
		req.RemoteAddr = "192.0.2.10:1234"
		req.Header.Set("X-Forwarded-For", "198.51.100.1")
		req.Header.Set("X-Real-IP", "203.0.113.1")
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusUnauthorized, rec.Code, rec.Body.String())
	}
	if len(lockout.keys) != 1 {
		t.Fatalf("expected one lockout key, got %d", len(lockout.keys))
	}
	key := lockout.keys[0]
	if !strings.HasPrefix(key, "ip:203.0.113.1|") {
		t.Fatalf("lockout ignored client IP header selection: %q", key)
	}
	if len(key) > 128 {
		t.Fatalf("lockout key length = %d, want <= 128", len(key))
	}
	if !strings.Contains(key, "|username-sha256:") {
		t.Fatalf("expected username hash in lockout key, got %q", key)
	}
	if strings.Contains(key, strings.Repeat("a", 128)) {
		t.Fatal("lockout key retained attacker-controlled username")
	}
}

func TestLoginLockoutFailuresHaveIdenticalResponses(t *testing.T) {
	var baseline *httptest.ResponseRecorder
	for _, policy := range []authcore.CapacityPolicy{authcore.AllowUntrackedKeys, authcore.RejectNewKeys} {
		for _, state := range []string{"empty", "full", "locked"} {
			lockout := authcore.NewMemoryLockout(authcore.MemoryLockoutOptions{MaxKeys: 1, MaxFailures: 2, CapacityPolicy: policy})
			if state == "full" {
				for range 2 {
					if _, _, err := lockout.RecordFailure(t.Context(), "unrelated"); err != nil {
						t.Fatal(err)
					}
				}
			}
			manager, err := authjwt.New("0123456789abcdef0123456789abcdef", authstore.NewMemoryStore())
			if err != nil {
				t.Fatal(err)
			}
			calls, events := 0, 0
			service, err := corerbac.NewService(corerbac.ServiceOptions{
				Users: newTestUserStore(corerbac.User{ID: "user-1", Username: "alice"}),
				Passwords: corerbac.PasswordVerifierFunc(func(_ context.Context, _ corerbac.User, password string) (bool, error) {
					calls++
					return password == "secret", nil
				}),
				Tokens: manager, Lockout: lockout,
				Events: corerbac.Events{OnLoginFailure: func(context.Context, corerbac.LoginFailure) error { events++; return nil }},
			})
			if err != nil {
				t.Fatal(err)
			}
			handler, err := rbachttp.NewHandler(service, rbachttp.Options{RateLimit: &rbachttp.RateLimitOptions{Disabled: true}})
			if err != nil {
				t.Fatal(err)
			}
			router := chi.NewRouter()
			if err := handler.Register(router); err != nil {
				t.Fatal(err)
			}
			login := func(password string) *httptest.ResponseRecorder {
				return serve(router, http.MethodPost, "/auth/login", `{"username":"alice","password":"`+password+`"}`, nil)
			}
			if state == "locked" {
				login("wrong")
				login("wrong")
				calls, events = 0, 0
			}
			if baseline == nil {
				baseline = login("wrong")
				events = 0
			}
			for _, password := range []string{"wrong", "", "wrong"} {
				rec := login(password)
				if rec.Code != http.StatusUnauthorized || rec.Body.String() != baseline.Body.String() || !reflect.DeepEqual(rec.Header(), baseline.Header()) {
					t.Fatalf("policy=%d state=%s leaked through response: %d %v %s", policy, state, rec.Code, rec.Header(), rec.Body.String())
				}
			}
			if events != 3 {
				t.Fatalf("policy=%d state=%s lost failure events: %d", policy, state, events)
			}
			if (state == "locked" || state == "full" && policy == authcore.RejectNewKeys) && calls != 0 {
				t.Fatalf("policy=%d state=%s reached verification %d times", policy, state, calls)
			}
			rec := login("secret")
			if state == "full" && policy == authcore.AllowUntrackedKeys {
				if rec.Code != http.StatusOK {
					t.Fatalf("capacity blocked valid credentials: %d %s", rec.Code, rec.Body.String())
				}
			} else if rec.Code != baseline.Code || rec.Body.String() != baseline.Body.String() || !reflect.DeepEqual(rec.Header(), baseline.Header()) {
				t.Fatalf("denied valid credentials revealed state: %d %v %s", rec.Code, rec.Header(), rec.Body.String())
			}
		}
	}
}

func TestLoginRateLimit(t *testing.T) {
	for _, tc := range []struct {
		name     string
		opts     rbachttp.Options
		wantLast int
	}{
		{"default", rbachttp.Options{}, http.StatusTooManyRequests},
		{"disabled", rbachttp.Options{RateLimit: &rbachttp.RateLimitOptions{Disabled: true}}, http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, router := newTestHandler(t, tc.opts)
			for i := range 6 {
				rec := serve(router, http.MethodPost, "/auth/login", `{"username":"alice","password":"wrong"}`, nil)
				want := http.StatusUnauthorized
				if i == 5 {
					want = tc.wantLast
				}
				if rec.Code != want {
					t.Fatalf("request %d: %d %s", i, rec.Code, rec.Body.String())
				}
			}
		})
	}
	for _, headers := range [][]string{nil, {}} {
		_, router := newTestHandler(t, rbachttp.Options{
			TrustedProxies:  []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")},
			ClientIPHeaders: []string{"X-Real-IP"},
			RateLimit:       &rbachttp.RateLimitOptions{Requests: 1, IPv4PrefixBits: 32, ClientIPHeaders: headers},
		})
		for i, realIP := range []string{"203.0.113.1", "203.0.113.2"} {
			rec := serve(router, http.MethodPost, "/auth/login", `{"username":"alice","password":"secret"}`, func(req *http.Request) {
				req.Header.Set("X-Real-IP", realIP)
				req.Header.Set("X-Forwarded-For", "198.51.100.1")
			})
			want := http.StatusOK
			if headers != nil && i == 1 {
				want = http.StatusTooManyRequests
			}
			if rec.Code != want {
				t.Fatalf("request %d: %d %s", i, rec.Code, rec.Body.String())
			}
		}
	}
}

func TestHandlerLogoutRejectsInvalidRefreshTokenAsUnauthorized(t *testing.T) {
	_, router := newTestHandler(t)

	rec := serve(router, http.MethodPost, "/auth/logout", `{"refresh_token":"not-a-jwt"}`, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusUnauthorized, rec.Code, rec.Body.String())
	}
}
