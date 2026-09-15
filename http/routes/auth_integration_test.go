package routes_test

import (
	"context"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Ithildur/EiluneKit/http/routes"
	"github.com/go-chi/chi/v5"
)

func TestMountApplicationAuthentication(t *testing.T) {
	type principalKey struct{}
	for _, test := range []struct {
		prefix string
		custom bool
	}{{"", false}, {"/api", false}, {"", true}, {"/api", true}} {
		name := test.prefix + "/default"
		if test.custom {
			name = test.prefix + "/custom"
		}
		t.Run(name, func(t *testing.T) {
			prefix := test.prefix
			authenticate := func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if cookie, err := r.Cookie("session"); err == nil && cookie.Value == "accepted" {
						ctx := context.WithValue(r.Context(), principalKey{}, "account-1")
						r = r.WithContext(routes.WithAuthenticated(ctx))
					}
					next.ServeHTTP(w, r)
				})
			}
			called := false
			api := routes.NewBlueprint(routes.DefaultMiddleware(authenticate))
			api.Get("/protected", "Protected", func(w http.ResponseWriter, r *http.Request) {
				called = true
				if r.Context().Value(principalKey{}) != "account-1" {
					t.Error("application principal was lost")
				}
				w.WriteHeader(http.StatusNoContent)
			}, routes.Auth(routes.AuthRequired))
			api.Get("/public", "Public", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}, routes.Auth(routes.AuthPublic))
			mux := chi.NewRouter()
			var unauthorized http.Handler
			if test.custom {
				unauthorized = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusUnauthorized)
					_ = json.MarshalWrite(w, map[string]any{"error": map[string]string{
						"code": "unauthorized", "request_id": r.Header.Get("X-Request-ID"),
					}})
				})
			}
			var err error
			if test.custom {
				err = routes.MountWithOptions(mux, prefix, api.Routes(), routes.MountOptions{Unauthorized: unauthorized})
			} else {
				err = routes.Mount(mux, prefix, api.Routes())
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, token := range []string{"", "invalid", "accepted"} {
				called = false
				request := httptest.NewRequest(http.MethodGet, prefix+"/protected", nil)
				request.Header.Set("X-Request-ID", "request-1")
				request.AddCookie(&http.Cookie{Name: "session", Value: token})
				recorder := httptest.NewRecorder()
				mux.ServeHTTP(recorder, request)
				if token == "accepted" {
					if recorder.Code != http.StatusNoContent || !called {
						t.Fatalf("authenticated request: status=%d called=%t", recorder.Code, called)
					}
					continue
				}
				var body struct {
					Code  string `json:"code"`
					Error struct {
						Code      string `json:"code"`
						RequestID string `json:"request_id"`
					} `json:"error"`
				}
				if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				validBody := body.Code == "unauthorized"
				if test.custom {
					validBody = body.Error.Code == "unauthorized" && body.Error.RequestID == "request-1"
				}
				if called || recorder.Code != http.StatusUnauthorized || !validBody {
					t.Fatalf("rejected request: status=%d called=%t body=%s", recorder.Code, called, recorder.Body)
				}
			}
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, prefix+"/public", nil))
			if recorder.Code != http.StatusNoContent {
				t.Fatalf("public endpoint was guarded: %d", recorder.Code)
			}
		})
	}
}
