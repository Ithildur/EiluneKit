package authhttp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	authcore "github.com/Ithildur/EiluneKit/auth"
	"github.com/Ithildur/EiluneKit/auth/http"
	authjwt "github.com/Ithildur/EiluneKit/auth/jwt"
	authsession "github.com/Ithildur/EiluneKit/auth/session"
	"github.com/Ithildur/EiluneKit/http/response"
	"github.com/Ithildur/EiluneKit/http/routes"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
)

type stubManager struct {
	issueAccess      string
	issueRefresh     string
	issueAccessExp   time.Time
	issueRefreshExp  time.Time
	issueErr         error
	issueLastUserID  string
	issueLastOptions authcore.IssueOptions

	rotateResult authcore.RefreshResult
	rotateOK     bool
	rotateErr    error

	revokeErr           error
	revokeSessionOK     bool
	revokeSessionErr    error
	revokeSessionUserID string
	revokeSessionID     string
	revokeAllErr        error
	revokeAllUserID     string

	validateAccessClaims authjwt.Claims
	validateAccessOK     bool
	validateAccessErr    error
	validateAccessCalls  int
	validateAccessToken  string
}

func (s *stubManager) IssueSessionTokens(ctx context.Context, userID string, opts authcore.IssueOptions) (string, time.Time, string, time.Time, error) {
	s.issueLastUserID = userID
	s.issueLastOptions = opts
	return s.issueAccess, s.issueAccessExp, s.issueRefresh, s.issueRefreshExp, s.issueErr
}

func (s *stubManager) RotateRefreshTokens(ctx context.Context, oldRefresh string) (authcore.RefreshResult, bool, error) {
	return s.rotateResult, s.rotateOK, s.rotateErr
}

func (s *stubManager) RevokeRefresh(ctx context.Context, refresh string) error {
	return s.revokeErr
}

func (s *stubManager) RevokeSession(ctx context.Context, userID, sessionID string) (bool, error) {
	s.revokeSessionUserID = userID
	s.revokeSessionID = sessionID
	return s.revokeSessionOK, s.revokeSessionErr
}

func (s *stubManager) RevokeAllSessions(ctx context.Context, userID string) error {
	s.revokeAllUserID = userID
	return s.revokeAllErr
}

func (s *stubManager) ValidateAccessToken(ctx context.Context, token string) (authjwt.Claims, bool, error) {
	s.validateAccessCalls++
	s.validateAccessToken = token
	if s.validateAccessOK {
		return s.validateAccessClaims, true, nil
	}
	return authjwt.Claims{}, false, s.validateAccessErr
}

func newRouter(t *testing.T, handler *authhttp.Handler) *chi.Mux {
	t.Helper()
	r := chi.NewRouter()
	if err := handler.Register(r); err != nil {
		t.Fatalf("register handler: %v", err)
	}
	return r
}

func mustNewHandler(t *testing.T, auth authhttp.TokenManager, opts authhttp.Options) *authhttp.Handler {
	t.Helper()
	handler, err := authhttp.NewHandler(auth, opts)
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}
	return handler
}

func mustNewTestRouter(t *testing.T, auth authhttp.TokenManager, opts authhttp.Options) *chi.Mux {
	t.Helper()
	return newRouter(t, mustNewHandler(t, auth, opts))
}

func mustNewStaticPasswordAuthenticator(t *testing.T, userID, password string) authhttp.LoginAuthenticator {
	t.Helper()
	authenticator, err := authhttp.NewStaticPasswordAuthenticator(userID, password)
	if err != nil {
		t.Fatalf("new static password authenticator: %v", err)
	}
	return authenticator
}

func stubAuthenticator(expectedUsername, expectedPassword, userID string) authhttp.LoginAuthenticator {
	return authhttp.LoginAuthenticatorFunc(func(ctx context.Context, username, password string) (string, bool, error) {
		return userID, username == expectedUsername && password == expectedPassword, nil
	})
}

func testOptions(authenticator authhttp.LoginAuthenticator) authhttp.Options {
	return authhttp.Options{
		LoginAuthenticator: authenticator,
		RateLimit:          &authhttp.RateLimitOptions{Disabled: true},
	}
}

func issuingManager(now time.Time) *stubManager {
	return &stubManager{
		issueAccess:     "access",
		issueRefresh:    "refresh",
		issueAccessExp:  now.Add(time.Hour),
		issueRefreshExp: now.Add(24 * time.Hour),
	}
}

func authenticatedManager() *stubManager {
	return &stubManager{
		validateAccessOK: true,
		validateAccessClaims: authjwt.Claims{
			SessionID:        "sid-1",
			RegisteredClaims: jwtRegisteredClaims("user-1"),
		},
	}
}

func serve(r http.Handler, method, path, body string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if mutate != nil {
		mutate(req)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestLogin(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		now := time.Now().UTC()
		manager := issuingManager(now)
		r := mustNewTestRouter(t, manager, testOptions(stubAuthenticator("admin", "secret", "user-1")))

		rec := serve(r, http.MethodPost, "/auth/login", `{"username":"admin","password":"secret","persistence":"persistent"}`, func(req *http.Request) {
			req.Header.Set("Content-Type", "application/json")
		})

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
		}
		if manager.issueLastUserID != "user-1" {
			t.Fatalf("expected issued user id user-1, got %q", manager.issueLastUserID)
		}

		var payload map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		if payload["access_token"] != "access" {
			t.Fatalf("expected access_token access, got %q", payload["access_token"])
		}
		if !hasCookie(rec.Result().Cookies(), authsession.DefaultRefreshCookieName) {
			t.Fatalf("expected refresh cookie")
		}
		if !hasCookie(rec.Result().Cookies(), authsession.DefaultCSRFCookieName) {
			t.Fatalf("expected csrf cookie")
		}
		assertPersistentCookie(t, cookieByName(rec.Result().Cookies(), authsession.DefaultRefreshCookieName))
		assertPersistentCookie(t, cookieByName(rec.Result().Cookies(), authsession.DefaultCSRFCookieName))
		if manager.issueLastOptions.SessionOnly {
			t.Fatal("expected persistent login to keep session_only disabled")
		}
	})

	t.Run("session_persistence", func(t *testing.T) {
		now := time.Now().UTC()
		manager := issuingManager(now)
		r := mustNewTestRouter(t, manager, testOptions(stubAuthenticator("admin", "secret", "user-1")))

		rec := serve(r, http.MethodPost, "/auth/login", `{"username":"admin","password":"secret","persistence":"session"}`, func(req *http.Request) {
			req.Header.Set("Content-Type", "application/json")
		})

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
		}
		if !manager.issueLastOptions.SessionOnly {
			t.Fatal("expected session persistence to propagate session_only")
		}
		assertSessionCookie(t, cookieByName(rec.Result().Cookies(), authsession.DefaultRefreshCookieName))
		assertSessionCookie(t, cookieByName(rec.Result().Cookies(), authsession.DefaultCSRFCookieName))
	})

	t.Run("invalid_credentials", func(t *testing.T) {
		r := mustNewTestRouter(t, &stubManager{}, testOptions(stubAuthenticator("admin", "secret", "user-1")))

		tests := []struct {
			name string
			body string
		}{
			{name: "wrong_password", body: `{"username":"admin","password":"wrong","persistence":"persistent"}`},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				rec := serve(r, http.MethodPost, "/auth/login", tc.body, func(req *http.Request) {
					req.Header.Set("Content-Type", "application/json")
				})
				assertErrorResponse(t, rec, http.StatusUnauthorized, "unauthorized", "invalid credentials")
			})
		}
	})

	t.Run("error_responses", func(t *testing.T) {
		tests := []struct {
			name       string
			build      func(t *testing.T) *authhttp.Handler
			body       string
			wantStatus int
			wantCode   string
			wantMsg    string
		}{
			{
				name: "invalid_json",
				build: func(t *testing.T) *authhttp.Handler {
					return mustNewHandler(t, &stubManager{}, testOptions(stubAuthenticator("admin", "secret", "user-1")))
				},
				body:       `{"username":"admin"`,
				wantStatus: http.StatusBadRequest,
				wantCode:   "invalid_json",
				wantMsg:    "invalid json",
			},
			{
				name: "body_too_large",
				build: func(t *testing.T) *authhttp.Handler {
					opts := testOptions(stubAuthenticator("admin", "secret", "user-1"))
					opts.MaxBodyBytes = 8
					return mustNewHandler(t, &stubManager{}, opts)
				},
				body:       `{"username":"admin","password":"secret","persistence":"persistent"}`,
				wantStatus: http.StatusRequestEntityTooLarge,
				wantCode:   "body_too_large",
				wantMsg:    "request body too large",
			},
			{
				name: "authenticator_error",
				build: func(t *testing.T) *authhttp.Handler {
					return mustNewHandler(t, &stubManager{}, testOptions(authhttp.LoginAuthenticatorFunc(func(ctx context.Context, username, password string) (string, bool, error) {
						return "", false, errors.New("backend unavailable")
					})))
				},
				body:       `{"username":"admin","password":"secret","persistence":"persistent"}`,
				wantStatus: http.StatusInternalServerError,
				wantCode:   "auth_error",
				wantMsg:    "auth failed",
			},
			{
				name: "authenticator_contract_violation",
				build: func(t *testing.T) *authhttp.Handler {
					return mustNewHandler(t, &stubManager{}, testOptions(authhttp.LoginAuthenticatorFunc(func(ctx context.Context, username, password string) (string, bool, error) {
						return "", true, nil
					})))
				},
				body:       `{"username":"admin","password":"secret","persistence":"persistent"}`,
				wantStatus: http.StatusInternalServerError,
				wantCode:   "auth_misconfigured",
				wantMsg:    "auth is misconfigured",
			},
			{
				name: "missing_persistence",
				build: func(t *testing.T) *authhttp.Handler {
					return mustNewHandler(t, &stubManager{}, testOptions(stubAuthenticator("admin", "secret", "user-1")))
				},
				body:       `{"username":"admin","password":"secret"}`,
				wantStatus: http.StatusBadRequest,
				wantCode:   "invalid_persistence",
				wantMsg:    "persistence must be session or persistent",
			},
			{
				name: "invalid_persistence",
				build: func(t *testing.T) *authhttp.Handler {
					return mustNewHandler(t, &stubManager{}, testOptions(stubAuthenticator("admin", "secret", "user-1")))
				},
				body:       `{"username":"admin","password":"secret","persistence":"weekly"}`,
				wantStatus: http.StatusBadRequest,
				wantCode:   "invalid_persistence",
				wantMsg:    "persistence must be session or persistent",
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				r := newRouter(t, tc.build(t))
				rec := serve(r, http.MethodPost, "/auth/login", tc.body, func(req *http.Request) {
					req.Header.Set("Content-Type", "application/json")
				})
				assertErrorResponse(t, rec, tc.wantStatus, tc.wantCode, tc.wantMsg)
			})
		}
	})
}

func TestRegisterRejectsNilHandler(t *testing.T) {
	var handler *authhttp.Handler

	err := handler.Register(chi.NewRouter())
	if err == nil {
		t.Fatal("expected nil handler to fail")
	}
	if got, want := err.Error(), "authhttp: handler is nil"; got != want {
		t.Fatalf("expected error %q, got %q", want, got)
	}
}

func TestRoutesExportAuthRequirement(t *testing.T) {
	handler := mustNewHandler(t, &stubManager{}, testOptions(stubAuthenticator("admin", "secret", "user-1")))
	routeList := handler.Routes()

	payload, err := routes.ExportJSON(routeList)
	if err != nil {
		t.Fatalf("export routes: %v", err)
	}

	var exported []struct {
		Path string                 `json:"path"`
		Auth routes.AuthRequirement `json:"auth"`
	}
	if err := json.Unmarshal(payload, &exported); err != nil {
		t.Fatalf("unmarshal routes: %v", err)
	}

	authByPath := make(map[string]routes.AuthRequirement, len(exported))
	for _, route := range exported {
		authByPath[route.Path] = route.Auth
	}
	if got, want := authByPath["/auth/login"], routes.AuthPublic; got != want {
		t.Fatalf("expected login auth %q, got %q", want, got)
	}
	if got, want := authByPath["/auth/refresh"], routes.AuthRequired; got != want {
		t.Fatalf("expected refresh auth %q, got %q", want, got)
	}
	if got, want := authByPath["/auth/sessions/current"], routes.AuthRequired; got != want {
		t.Fatalf("expected session auth %q, got %q", want, got)
	}
}

func TestRefreshCookiePath(t *testing.T) {
	tests := []struct {
		name            string
		options         authhttp.Options
		wantRefreshPath string
	}{
		{
			name: "defaults_to_base_path",
			options: authhttp.Options{
				LoginAuthenticator: stubAuthenticator("admin", "secret", "user-1"),
				RateLimit:          &authhttp.RateLimitOptions{Disabled: true},
			},
			wantRefreshPath: "/auth",
		},
		{
			name: "uses_explicit_public_path",
			options: authhttp.Options{
				BasePath:           "/auth",
				RefreshCookiePath:  "/api/auth",
				LoginAuthenticator: stubAuthenticator("admin", "secret", "user-1"),
				RateLimit:          &authhttp.RateLimitOptions{Disabled: true},
			},
			wantRefreshPath: "/api/auth",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now().UTC()
			manager := issuingManager(now)
			r := mustNewTestRouter(t, manager, tc.options)

			rec := serve(r, http.MethodPost, "/auth/login", `{"username":"admin","password":"secret","persistence":"persistent"}`, func(req *http.Request) {
				req.Header.Set("Content-Type", "application/json")
			})

			refresh := cookieByName(rec.Result().Cookies(), authsession.DefaultRefreshCookieName)
			if refresh == nil {
				t.Fatal("expected refresh cookie")
			}
			if got := refresh.Path; got != tc.wantRefreshPath {
				t.Fatalf("expected refresh cookie path %q, got %q", tc.wantRefreshPath, got)
			}
		})
	}
}

func TestRefresh(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		now := time.Now().UTC()
		manager := &stubManager{
			rotateResult: authcore.RefreshResult{
				Access:           "newaccess",
				Refresh:          "newrefresh",
				AccessExpiresAt:  now.Add(time.Hour),
				RefreshExpiresAt: now.Add(24 * time.Hour),
			},
			rotateOK: true,
		}
		r := mustNewTestRouter(t, manager, testOptions(mustNewStaticPasswordAuthenticator(t, "dashboard-admin", "secret")))

		rec := serve(r, http.MethodPost, "/auth/refresh", "", func(req *http.Request) {
			req.AddCookie(&http.Cookie{Name: authsession.DefaultRefreshCookieName, Value: "oldrefresh"})
			req.AddCookie(&http.Cookie{Name: authsession.DefaultCSRFCookieName, Value: "csrf"})
			req.Header.Set(authsession.DefaultCSRFHeaderName, "csrf")
		})

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
		}

		var payload map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		if payload["access_token"] != "newaccess" {
			t.Fatalf("expected access_token newaccess, got %q", payload["access_token"])
		}
		if !hasCookie(rec.Result().Cookies(), authsession.DefaultRefreshCookieName) {
			t.Fatalf("expected refresh cookie")
		}
		assertPersistentCookie(t, cookieByName(rec.Result().Cookies(), authsession.DefaultRefreshCookieName))
		assertPersistentCookie(t, cookieByName(rec.Result().Cookies(), authsession.DefaultCSRFCookieName))
	})

	t.Run("session_only", func(t *testing.T) {
		now := time.Now().UTC()
		manager := &stubManager{
			rotateResult: authcore.RefreshResult{
				Access:           "newaccess",
				Refresh:          "newrefresh",
				AccessExpiresAt:  now.Add(time.Hour),
				RefreshExpiresAt: now.Add(24 * time.Hour),
				SessionOnly:      true,
			},
			rotateOK: true,
		}
		r := mustNewTestRouter(t, manager, testOptions(mustNewStaticPasswordAuthenticator(t, "dashboard-admin", "secret")))

		rec := serve(r, http.MethodPost, "/auth/refresh", "", func(req *http.Request) {
			req.AddCookie(&http.Cookie{Name: authsession.DefaultRefreshCookieName, Value: "oldrefresh"})
			req.AddCookie(&http.Cookie{Name: authsession.DefaultCSRFCookieName, Value: "csrf"})
			req.Header.Set(authsession.DefaultCSRFHeaderName, "csrf")
		})

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
		}
		assertSessionCookie(t, cookieByName(rec.Result().Cookies(), authsession.DefaultRefreshCookieName))
		assertSessionCookie(t, cookieByName(rec.Result().Cookies(), authsession.DefaultCSRFCookieName))
	})

	t.Run("error_responses", func(t *testing.T) {
		tests := []struct {
			name       string
			manager    *stubManager
			cookies    []*http.Cookie
			csrfHeader string
			wantStatus int
			wantCode   string
			wantMsg    string
		}{
			{
				name: "csrf_mismatch",
				cookies: []*http.Cookie{
					{Name: authsession.DefaultRefreshCookieName, Value: "oldrefresh"},
					{Name: authsession.DefaultCSRFCookieName, Value: "cookie-csrf"},
				},
				csrfHeader: "header-csrf",
				wantStatus: http.StatusUnauthorized,
				wantCode:   "unauthorized",
				wantMsg:    "csrf validation failed",
			},
			{
				name:    "missing_refresh_cookie",
				manager: &stubManager{},
				cookies: []*http.Cookie{
					{Name: authsession.DefaultCSRFCookieName, Value: "csrf"},
				},
				csrfHeader: "csrf",
				wantStatus: http.StatusUnauthorized,
				wantCode:   "unauthorized",
				wantMsg:    "refresh token missing",
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				manager := tc.manager
				if manager == nil {
					manager = &stubManager{}
				}
				r := mustNewTestRouter(t, manager, testOptions(mustNewStaticPasswordAuthenticator(t, "dashboard-admin", "secret")))
				rec := serve(r, http.MethodPost, "/auth/refresh", "", func(req *http.Request) {
					for _, cookie := range tc.cookies {
						req.AddCookie(cookie)
					}
					req.Header.Set(authsession.DefaultCSRFHeaderName, tc.csrfHeader)
				})
				assertErrorResponse(t, rec, tc.wantStatus, tc.wantCode, tc.wantMsg)
			})
		}
	})
}

func TestSessionEndpoints(t *testing.T) {
	t.Run("logout_clears_cookies", func(t *testing.T) {
		r := mustNewTestRouter(t, &stubManager{}, testOptions(stubAuthenticator("admin", "secret", "user-1")))

		rec := serve(r, http.MethodPost, "/auth/logout", "", func(req *http.Request) {
			req.AddCookie(&http.Cookie{Name: authsession.DefaultRefreshCookieName, Value: "refresh"})
			req.AddCookie(&http.Cookie{Name: authsession.DefaultCSRFCookieName, Value: "csrf"})
			req.Header.Set(authsession.DefaultCSRFHeaderName, "csrf")
		})

		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
		}
		if !hasCookie(rec.Result().Cookies(), authsession.DefaultRefreshCookieName) {
			t.Fatalf("expected refresh cookie")
		}
		if !hasCookie(rec.Result().Cookies(), authsession.DefaultCSRFCookieName) {
			t.Fatalf("expected csrf cookie")
		}
	})

	t.Run("delete_current_session", func(t *testing.T) {
		manager := authenticatedManager()
		manager.revokeSessionOK = true
		r := mustNewTestRouter(t, manager, testOptions(stubAuthenticator("admin", "secret", "user-1")))

		rec := serve(r, http.MethodDelete, "/auth/sessions/current", "", func(req *http.Request) {
			req.Header.Set("Authorization", "Bearer access")
		})

		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
		}
		if manager.revokeSessionUserID != "user-1" || manager.revokeSessionID != "sid-1" {
			t.Fatalf("unexpected revoke session call: user=%q sid=%q", manager.revokeSessionUserID, manager.revokeSessionID)
		}
		if got, want := manager.validateAccessCalls, 1; got != want {
			t.Fatalf("expected %d access token validation, got %d", want, got)
		}
		if got, want := manager.validateAccessToken, "access"; got != want {
			t.Fatalf("expected token %q, got %q", want, got)
		}
	})

	t.Run("delete_all_sessions", func(t *testing.T) {
		manager := authenticatedManager()
		r := mustNewTestRouter(t, manager, testOptions(stubAuthenticator("admin", "secret", "user-1")))

		rec := serve(r, http.MethodDelete, "/auth/sessions", "", func(req *http.Request) {
			req.Header.Set("Authorization", "Bearer access")
		})

		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
		}
		if manager.revokeAllUserID != "user-1" {
			t.Fatalf("expected revoke-all for user-1, got %q", manager.revokeAllUserID)
		}
	})

	t.Run("delete_specific_current_session_clears_cookies", func(t *testing.T) {
		manager := authenticatedManager()
		manager.revokeSessionOK = true
		r := mustNewTestRouter(t, manager, testOptions(stubAuthenticator("admin", "secret", "user-1")))

		rec := serve(r, http.MethodDelete, "/auth/sessions/sid-1", "", func(req *http.Request) {
			req.Header.Set("Authorization", "Bearer access")
		})

		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
		}
		if !hasCookie(rec.Result().Cookies(), authsession.DefaultRefreshCookieName) {
			t.Fatalf("expected refresh cookie clear")
		}
	})

	t.Run("error_responses", func(t *testing.T) {
		tests := []struct {
			name       string
			manager    *stubManager
			path       string
			wantStatus int
			wantCode   string
			wantMsg    string
		}{
			{
				name: "missing_session_id",
				manager: &stubManager{
					validateAccessOK: true,
					validateAccessClaims: authjwt.Claims{
						SessionID:        "sid-1",
						RegisteredClaims: jwtRegisteredClaims("user-1"),
					},
				},
				path:       "/auth/sessions/%20",
				wantStatus: http.StatusBadRequest,
				wantCode:   "invalid_session",
				wantMsg:    "session id is required",
			},
			{
				name: "validator_unavailable",
				manager: &stubManager{
					validateAccessErr: authjwt.ErrStoreUnavailable,
				},
				path:       "/auth/sessions/current",
				wantStatus: http.StatusServiceUnavailable,
				wantCode:   "auth_unavailable",
				wantMsg:    "auth is unavailable",
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				r := mustNewTestRouter(t, tc.manager, testOptions(stubAuthenticator("admin", "secret", "user-1")))
				rec := serve(r, http.MethodDelete, tc.path, "", func(req *http.Request) {
					req.Header.Set("Authorization", "Bearer access")
				})
				assertErrorResponse(t, rec, tc.wantStatus, tc.wantCode, tc.wantMsg)
			})
		}
	})
}

func TestLoginRateLimit(t *testing.T) {
	tests := []struct {
		name    string
		options authhttp.Options
		run     func(t *testing.T, r http.Handler)
	}{
		{
			name: "ignores_spoofed_forwarded_headers_by_default",
			options: authhttp.Options{
				LoginAuthenticator: stubAuthenticator("admin", "secret", "user-1"),
			},
			run: func(t *testing.T, r http.Handler) {
				var lastCode int
				for i := 0; i < 6; i++ {
					rec := serve(r, http.MethodPost, "/auth/login", `{"username":"admin","password":"secret","persistence":"persistent"}`, func(req *http.Request) {
						req.Header.Set("Content-Type", "application/json")
						req.Header.Set("X-Forwarded-For", "198.51.100.10, 198.51.100.11")
						req.RemoteAddr = "192.0.2.1:1234"
					})
					lastCode = rec.Code
				}
				if lastCode != http.StatusTooManyRequests {
					t.Fatalf("expected status %d, got %d", http.StatusTooManyRequests, lastCode)
				}
			},
		},
		{
			name: "trusted_proxy_uses_forwarded_headers",
			options: authhttp.Options{
				LoginAuthenticator: stubAuthenticator("admin", "secret", "user-1"),
				RateLimit: &authhttp.RateLimitOptions{
					Requests:       1,
					Window:         time.Minute,
					IPv4PrefixBits: 32,
					TrustedProxies: []netip.Prefix{mustPrefix(t, "192.0.2.0/24")},
				},
			},
			run: func(t *testing.T, r http.Handler) {
				request := func(xff string) int {
					return serve(r, http.MethodPost, "/auth/login", `{"username":"admin","password":"secret","persistence":"persistent"}`, func(req *http.Request) {
						req.Header.Set("Content-Type", "application/json")
						req.Header.Set("X-Forwarded-For", xff)
						req.RemoteAddr = "192.0.2.10:1234"
					}).Code
				}

				if got := request("198.51.100.7"); got != http.StatusOK {
					t.Fatalf("expected first trusted proxy request to succeed, got %d", got)
				}
				if got := request("198.51.100.8"); got != http.StatusOK {
					t.Fatalf("expected second trusted proxy request with different forwarded ip to succeed, got %d", got)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.run(t, mustNewTestRouter(t, issuingManager(time.Now().UTC()), tc.options))
		})
	}
}

func jwtRegisteredClaims(userID string) jwt.RegisteredClaims {
	return jwt.RegisteredClaims{Subject: userID}
}

func hasCookie(cookies []*http.Cookie, name string) bool {
	return cookieByName(cookies, name) != nil
}

func cookieByName(cookies []*http.Cookie, name string) *http.Cookie {
	for _, c := range cookies {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func mustPrefix(t *testing.T, raw string) netip.Prefix {
	t.Helper()
	prefix, err := netip.ParsePrefix(raw)
	if err != nil {
		t.Fatalf("parse prefix %q: %v", raw, err)
	}
	return prefix
}

func assertPersistentCookie(t *testing.T, cookie *http.Cookie) {
	t.Helper()
	if cookie == nil {
		t.Fatal("expected cookie")
	}
	if cookie.MaxAge <= 0 {
		t.Fatalf("expected persistent cookie MaxAge > 0, got %d", cookie.MaxAge)
	}
	if cookie.Expires.IsZero() {
		t.Fatal("expected persistent cookie expiration")
	}
}

func assertSessionCookie(t *testing.T, cookie *http.Cookie) {
	t.Helper()
	if cookie == nil {
		t.Fatal("expected cookie")
	}
	if cookie.MaxAge != 0 {
		t.Fatalf("expected session cookie MaxAge=0, got %d", cookie.MaxAge)
	}
	if !cookie.Expires.IsZero() {
		t.Fatalf("expected session cookie without expiration, got %s", cookie.Expires.UTC().Format(time.RFC3339))
	}
}

func assertErrorResponse(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantCode, wantMsg string) {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("expected status %d, got %d", wantStatus, rec.Code)
	}

	var payload response.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	if payload.Code != wantCode {
		t.Fatalf("expected code %q, got %q", wantCode, payload.Code)
	}
	if payload.Message != wantMsg {
		t.Fatalf("expected message %q, got %q", wantMsg, payload.Message)
	}
}
