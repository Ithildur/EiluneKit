package captchahttp_test

import (
	"context"
	"encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ithildur/EiluneKit/auth"
	authhttp "github.com/Ithildur/EiluneKit/auth/http"
	"github.com/Ithildur/EiluneKit/auth/jwt"
	"github.com/Ithildur/EiluneKit/auth/rbac"
	rbachttp "github.com/Ithildur/EiluneKit/auth/rbac/http"
	"github.com/Ithildur/EiluneKit/auth/store"
	"github.com/Ithildur/EiluneKit/captcha"
	captchahttp "github.com/Ithildur/EiluneKit/captcha/http"
	"github.com/Ithildur/EiluneKit/http/routes"
	"github.com/Ithildur/EiluneKit/tools/openapi"
	"github.com/go-chi/chi/v5"
)

type authHandler interface {
	Register(chi.Router) error
	Routes() []routes.Route
}

type loginState struct {
	passwordChecks int
	lockoutCalls   int
	sessions       *store.MemoryStore
}

func (s *loginState) Check(context.Context, string) (time.Time, bool, error) {
	s.lockoutCalls++
	return time.Time{}, false, nil
}
func (s *loginState) RecordFailure(context.Context, string) (time.Time, bool, error) {
	s.lockoutCalls++
	return time.Time{}, false, nil
}
func (s *loginState) Clear(context.Context, string) error { s.lockoutCalls++; return nil }

type userStore struct{}

func (userStore) GetUser(context.Context, string) (rbac.User, bool, error) {
	return rbac.User{ID: "user-1", Username: "alice"}, true, nil
}
func (s userStore) GetUserByUsername(ctx context.Context, name string) (rbac.User, bool, error) {
	return s.GetUser(ctx, name)
}

func makeAuthHandler(t *testing.T, kind string, options captchahttp.Options, rate *authhttp.RateLimitOptions, maxBody int64) (authHandler, *loginState, error) {
	t.Helper()
	s := &loginState{sessions: store.NewMemoryStore()}
	manager, err := jwt.New("0123456789abcdef0123456789abcdef", s.sessions)
	if err != nil {
		t.Fatal(err)
	}
	if kind == "session" {
		h, err := authhttp.NewHandler(manager, authhttp.Options{
			BasePath: new("/tenants/{tenantID}/auth"), RateLimit: rate, MaxBodyBytes: maxBody,
			Captcha: options, LoginLockout: s,
			LoginAuthenticator: auth.LoginAuthenticatorFunc(func(context.Context, string, string) (string, bool, error) {
				s.passwordChecks++
				return "user-1", true, nil
			}),
		})
		return h, s, err
	}
	service, err := rbac.NewService(rbac.ServiceOptions{
		Users: userStore{}, Tokens: manager, Lockout: s,
		Passwords: rbac.PasswordVerifierFunc(func(context.Context, rbac.User, string) (bool, error) {
			s.passwordChecks++
			return true, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	h, err := rbachttp.NewHandler(service, rbachttp.Options{
		BasePath: new("/tenants/{tenantID}/auth"), RateLimit: rate, MaxBodyBytes: maxBody, Captcha: options,
	})
	return h, s, err
}

func loginRequest(token, contentType string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/tenants/acme/auth/login", strings.NewReader(`{"username":"alice","password":"secret","persistence":"session"}`))
	r.Header.Set("Content-Type", contentType)
	r.Header.Set(captchahttp.TokenHeader, token)
	return r
}

func TestAuthLoginCaptcha(t *testing.T) {
	for _, kind := range []string{"session", "rbac"} {
		t.Run(kind, func(t *testing.T) {
			if _, _, err := makeAuthHandler(t, kind, captchahttp.Options{Enabled: true}, nil, 0); !errors.Is(err, captcha.ErrMisconfigured) {
				t.Fatalf("enabled without verifier: %v", err)
			}
			for _, tt := range []struct {
				name               string
				enabled            bool
				token, contentType string
				upstreamStatus     int
				upstreamBody       string
				status, calls      int
				maxBody            int64
			}{
				{"disabled", false, "", "application/json", 403, `{"success":false,"error":"Invalid site key or secret"}`, 200, 0, 0},
				{"success", true, "token", "application/json", 200, `{"success":true}`, 200, 1, 0},
				{"missing", true, "", "application/json", 200, `{"success":true}`, 400, 0, 0},
				{"rejected", true, "token", "application/json", 403, `{"success":false,"error":"Token expired"}`, 403, 1, 0},
				{"bad_secret", true, "token", "application/json", 403, `{"success":false,"error":"Invalid site key or secret"}`, 500, 1, 0},
				{"unavailable", true, "token", "application/json", 503, `unavailable`, 503, 1, 0},
				{"content_type", true, "token", "text/plain", 200, `{"success":true}`, 415, 0, 0},
				{"body_limit", true, "token", "application/json", 200, `{"success":true}`, 413, 1, 8},
			} {
				t.Run(tt.name, func(t *testing.T) {
					var calls atomic.Int32
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						w.WriteHeader(tt.upstreamStatus)
						_, _ = io.WriteString(w, tt.upstreamBody)
					}))
					defer server.Close()
					client, err := captcha.NewCap(server.URL+"/site/siteverify", "secret", captcha.Options{})
					if err != nil {
						t.Fatal(err)
					}
					h, state, err := makeAuthHandler(t, kind, captchahttp.Options{Enabled: tt.enabled, Verify: client.Verify}, &authhttp.RateLimitOptions{Disabled: true}, tt.maxBody)
					if err != nil {
						t.Fatal(err)
					}
					for _, mountDirectly := range []bool{false, true} {
						r := chi.NewRouter()
						if mountDirectly {
							err = routes.Mount(r, "", h.Routes())
						} else {
							err = h.Register(r)
						}
						if err != nil {
							t.Fatal(err)
						}
						w := httptest.NewRecorder()
						r.ServeHTTP(w, loginRequest(tt.token, tt.contentType))
						if w.Code != tt.status {
							t.Fatalf("status=%d, want %d, body=%s", w.Code, tt.status, w.Body.String())
						}
						if tt.status != 200 && (len(w.Result().Cookies()) != 0 || strings.Contains(w.Body.String(), "access_token")) {
							t.Fatal("failed login issued credentials")
						}
						// Other auth endpoints must not invoke CAPTCHA, even without a token.
						// 其他认证端点即使未携带 token 也不应调用 CAPTCHA。
						for _, route := range h.Routes() {
							if strings.HasSuffix(route.Path, "/login") {
								continue
							}
							path := strings.ReplaceAll(strings.ReplaceAll(route.Path, "{tenantID}", "acme"), "{sid}", "sid")
							request := httptest.NewRequest(route.Method, path, strings.NewReader(`{}`))
							request.Header.Set("Content-Type", "application/json")
							r.ServeHTTP(httptest.NewRecorder(), request)
						}
						assertCaptchaContract(t, h.Routes(), tt.enabled)
					}
					if calls.Load() != int32(tt.calls*2) {
						t.Fatalf("verification calls=%d, want %d", calls.Load(), tt.calls*2)
					}
					sessions, err := state.sessions.Sessions(t.Context(), "user-1")
					if err != nil {
						t.Fatal(err)
					}
					if tt.status == 200 {
						if state.passwordChecks != 2 || len(sessions) != 2 {
							t.Fatalf("login did not complete: checks=%d sessions=%d", state.passwordChecks, len(sessions))
						}
					} else if state.passwordChecks != 0 || state.lockoutCalls != 0 || len(sessions) != 0 {
						t.Fatalf("failure reached auth: checks=%d lockout=%d sessions=%d", state.passwordChecks, state.lockoutCalls, len(sessions))
					}
				})
			}
		})
	}
}

func TestLoginRateLimitPrecedesCaptcha(t *testing.T) {
	for _, kind := range []string{"session", "rbac"} {
		t.Run(kind, func(t *testing.T) {
			calls := 0
			h, _, err := makeAuthHandler(t, kind, captchahttp.Options{Enabled: true, Verify: func(context.Context, string) error { calls++; return captcha.ErrRejected }}, &authhttp.RateLimitOptions{Requests: 1, Window: time.Minute}, 0)
			if err != nil {
				t.Fatal(err)
			}
			r := chi.NewRouter()
			if err := h.Register(r); err != nil {
				t.Fatal(err)
			}
			for _, status := range []int{403, 429} {
				w := httptest.NewRecorder()
				r.ServeHTTP(w, loginRequest("token", "application/json"))
				if w.Code != status {
					t.Fatalf("status=%d, want %d", w.Code, status)
				}
			}
			if calls != 1 {
				t.Fatalf("rate-limited request reached CAPTCHA: %d", calls)
			}
		})
	}
}

func assertCaptchaContract(t *testing.T, routeList []routes.Route, enabled bool) {
	t.Helper()
	payload, err := openapi.Generate(routeList, openapi.Options{Title: "Auth", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Paths map[string]map[string]struct {
			Parameters []struct {
				Name     string `json:"name"`
				In       string `json:"in"`
				Required bool   `json:"required"`
			} `json:"parameters"`
			Responses map[string]any `json:"responses"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatal(err)
	}
	if _, ok := document.Paths["/tenants/{tenantID}/auth/login"]["post"]; !ok {
		t.Fatal("login operation missing from generated contract")
	}
	for path, methods := range document.Paths {
		for _, operation := range methods {
			want := enabled && strings.HasSuffix(path, "/login")
			found := false
			for _, parameter := range operation.Parameters {
				if parameter.Name == captchahttp.TokenHeader {
					found = true
					if parameter.In != "header" || !parameter.Required {
						t.Fatal("CAPTCHA header must be required")
					}
				}
			}
			if found != want {
				t.Fatalf("CAPTCHA contract for %s: got %v, want %v", path, found, want)
			}
			if strings.HasSuffix(path, "/login") {
				_, forbidden := operation.Responses["403"]
				if forbidden != enabled {
					t.Fatal("403 contract does not follow CAPTCHA enablement")
				}
			}
		}
	}
}
