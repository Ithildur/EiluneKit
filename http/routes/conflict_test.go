package routes_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ithildur/EiluneKit/http/routes"
	"github.com/go-chi/chi/v5"
)

func TestMountRejectsConflictingRoutes(t *testing.T) {
	for _, test := range []struct{ first, second string }{
		{"/users", "/users"},
		{"/users/{id}", "/users/{name}"},
		{"/users/{id}/posts", "/users/{name}/profile"},
		{"/files/*", "/files/download"},
		{"/files/*", "/files/{name}"},
		{"/files/*", "/files/"},
		{"/files/*", "/files/*"},
		{"/files*", "/files"},
		{"/files*", "/files/readme"},
		{"/files*", "/files.txt"},
		{"/users/{id:[0-9]{2}}", "/users/{name:^[0-9]{2}$}"},
		{"/users/{:[0-9]+}", "/users/{:[0-9]+}"},
	} {
		for _, reverse := range []bool{false, true} {
			for _, separate := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/reverse=%v/separate=%v", test.first, test.second, reverse, separate), func(t *testing.T) {
					first, second := test.first, test.second
					if reverse {
						first, second = second, first
					}
					mux := chi.NewRouter()
					original := routes.Route{Method: "get", Path: first, Handler: http.NotFoundHandler()}
					if separate {
						if err := routes.Mount(mux, "api", []routes.Route{original}); err != nil {
							t.Fatal(err)
						}
					}
					wrapped := false
					batch := []routes.Route{{Method: http.MethodGet, Path: "/new", Handler: http.NotFoundHandler(), Middleware: []routes.Middleware{
						func(next http.Handler) http.Handler { wrapped = true; return next },
					}}}
					if !separate {
						batch = append(batch, original)
					}
					batch = append(batch, routes.Route{Method: http.MethodGet, Path: second, Handler: http.NotFoundHandler()})
					assertRouteConflict(t, mux, "api", batch, "GET", "/api"+first, "/api"+second)
					if wrapped {
						t.Fatal("middleware was constructed before conflict detection completed")
					}
					if mux.Match(chi.NewRouteContext(), http.MethodGet, "/api/new") {
						t.Fatal("failed mount registered part of the batch")
					}
				})
			}
		}
	}
}

func TestMountCannotOverwriteAuthentication(t *testing.T) {
	mux := chi.NewRouter()
	private := routes.NewBlueprint(routes.DefaultAuth(routes.AuthRequired))
	private.Get("/account", "", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	if err := private.MountAt(mux, "api"); err != nil {
		t.Fatal(err)
	}
	public := routes.NewBlueprint()
	public.Get("/account", "", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusAccepted) })
	assertRouteConflict(t, mux, "api", public.Routes(), "GET", "/api/account")
	for _, authenticated := range []bool{false, true} {
		request := httptest.NewRequest(http.MethodGet, "/api/account", nil)
		want := http.StatusUnauthorized
		if authenticated {
			request = request.WithContext(routes.WithAuthenticated(request.Context()))
			want = http.StatusNoContent
		}
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, request)
		if recorder.Code != want {
			t.Fatalf("authenticated=%v: status %d, want %d", authenticated, recorder.Code, want)
		}
	}
}

func TestMountChecksExistingChiRoutes(t *testing.T) {
	for _, setup := range []string{"direct", "inline", "all methods", "subrouter", "opaque mount"} {
		t.Run(setup, func(t *testing.T) {
			mux := chi.NewRouter()
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusAccepted) })
			switch setup {
			case "direct":
				mux.Get("/api/users", handler)
			case "inline":
				mux.With(func(next http.Handler) http.Handler { return next }).Get("/api/users", handler)
			case "all methods":
				mux.Handle("/api/users", handler)
			case "subrouter":
				mux.Route("/api", func(r chi.Router) { r.Get("/users", handler) })
			case "opaque mount":
				mux.Mount("/api", handler)
			}
			assertRouteConflict(t, mux, "api", []routes.Route{{Method: http.MethodGet, Path: "/users", Handler: http.NotFoundHandler()}}, "GET", "/api/users")
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/users", nil))
			if recorder.Code != http.StatusAccepted {
				t.Fatalf("original handler lost: status %d", recorder.Code)
			}
		})
	}
}

func TestMountUsesEffectiveMethodRegistrations(t *testing.T) {
	mux := chi.NewRouter()
	mux.Handle("/users/{id}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "all/"+r.PathValue("id"))
	}))
	mux.Get("/users/{name}", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "get/"+r.PathValue("name"))
	})
	api := routes.NewBlueprint()
	api.Get("/users/{name}/posts", "", func(w http.ResponseWriter, r *http.Request, name string) {
		_, _ = fmt.Fprint(w, "posts/"+name)
	})
	if err := api.Mount(mux); err != nil {
		t.Fatal(err)
	}
	assertRouteConflict(t, mux, "", []routes.Route{{Method: http.MethodPost, Path: "/users/{name}/posts", Handler: http.NotFoundHandler()}}, "POST", "/users/{id}")

	for _, test := range []struct{ method, path, want string }{
		{http.MethodGet, "/users/alice", "get/alice"},
		{http.MethodPost, "/users/alice", "all/alice"},
		{http.MethodGet, "/users/alice/posts", "posts/alice"},
	} {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequest(test.method, test.path, nil))
		if recorder.Code != http.StatusOK || recorder.Body.String() != test.want {
			t.Errorf("%s %s: status %d, body %q; want 200, %q", test.method, test.path, recorder.Code, recorder.Body.String(), test.want)
		}
	}
}

func TestMountUsesEffectiveSubrouterMethods(t *testing.T) {
	for _, test := range []struct{ mounted, overridden string }{
		{"/users/{id}", "/users/{name}"},
		{"/users/{id:[0-9]{2}}", "/users/{name:^[0-9]{2}$}"},
	} {
		t.Run(test.mounted, func(t *testing.T) {
			mux := chi.NewRouter()
			sub := chi.NewRouter()
			sub.Handle("/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = fmt.Fprint(w, "sub/"+r.PathValue("id"))
			}))
			mux.Mount(test.mounted+"/", sub)
			mux.Get(test.overridden+"/*", func(w http.ResponseWriter, r *http.Request) {
				_, _ = fmt.Fprint(w, "override/"+r.PathValue("name"))
			})
			api := routes.NewBlueprint()
			api.Get(test.overridden, "", func(w http.ResponseWriter, r *http.Request, name string) {
				_, _ = fmt.Fprint(w, "get/"+name)
			})
			api.Post(test.mounted, "", func(w http.ResponseWriter, r *http.Request, id string) {
				_, _ = fmt.Fprint(w, "post/"+id)
			})
			if err := api.Mount(mux); err != nil {
				t.Fatal(err)
			}
			for _, route := range []struct{ method, path string }{
				{http.MethodGet, test.overridden + "/posts"},
				{http.MethodPost, test.mounted + "/posts"},
			} {
				assertRouteConflict(t, mux, "", []routes.Route{{Method: route.method, Path: route.path, Handler: http.NotFoundHandler()}}, route.method, "catch-all")
			}
			for _, request := range []struct{ method, path, want string }{
				{http.MethodGet, "/users/42", "get/42"},
				{http.MethodPost, "/users/42", "post/42"},
				{http.MethodGet, "/users/42/posts", "override/42"},
				{http.MethodPost, "/users/42/posts", "sub/42"},
			} {
				recorder := httptest.NewRecorder()
				mux.ServeHTTP(recorder, httptest.NewRequest(request.method, request.path, nil))
				if recorder.Code != http.StatusOK || recorder.Body.String() != request.want {
					t.Errorf("%s %s: status %d, body %q; want 200, %q", request.method, request.path, recorder.Code, recorder.Body.String(), request.want)
				}
			}
		})
	}
}

func TestMountAllowsDistinctRoutes(t *testing.T) {
	mux := chi.NewRouter()
	tests := []struct{ method, pattern, target string }{
		{http.MethodGet, "/users/{id}", "/users/alice"},
		{http.MethodGet, "/users/me", "/users/me"},
		{http.MethodPost, "/users/{name}", "/users/alice"},
		{http.MethodGet, "/users/{id}/posts", "/users/alice/posts"},
		{http.MethodGet, "/users/{id}/profile", "/users/alice/profile"},
		{http.MethodGet, "/users/{id:[0-9]{2}}", "/users/42"},
		{http.MethodGet, "/users/{name:[a-z]{2}}", "/users/ab"},
		{http.MethodGet, "/files", "/files"},
		{http.MethodGet, "/files/*", "/files/path/to/file"},
		{http.MethodPost, "/files/download", "/files/download"},
		{http.MethodGet, "/anonymous/{:[0-9]+}", "/anonymous/42"},
	}
	for _, test := range tests {
		if err := routes.Mount(mux, "api", []routes.Route{{Method: test.method, Path: test.pattern,
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = fmt.Fprint(w, test.pattern) }),
		}}); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range tests {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequest(test.method, "/api"+test.target, nil))
		if recorder.Code != http.StatusOK || recorder.Body.String() != test.pattern {
			t.Errorf("%s %s: status %d, body %q; want 200, %q", test.method, test.target, recorder.Code, recorder.Body.String(), test.pattern)
		}
	}
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/anonymous/alice", nil))
	if recorder.Code != http.StatusNotFound {
		t.Errorf("anonymous regex accepted a nonnumeric value: status %d", recorder.Code)
	}
}

func TestMountAllowsDirectoryBesideExternalCatchAll(t *testing.T) {
	for _, prefix := range []string{"/api", "/tenants/{id:[0-9]+}"} {
		for _, setup := range []string{"catch-all", "opaque mount", "subrouter"} {
			t.Run(prefix+"/"+setup, func(t *testing.T) {
				mux := chi.NewRouter()
				handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusAccepted) })
				switch setup {
				case "catch-all":
					mux.Handle(prefix+"/*", handler)
				case "opaque mount":
					mux.Mount(prefix+"/", handler)
				case "subrouter":
					sub := chi.NewRouter()
					mux.Mount(prefix+"/", sub)
					if err := routes.Mount(sub, "", []routes.Route{{Method: http.MethodGet, Path: "/users", Handler: handler}}); err != nil {
						t.Fatal(err)
					}
				}
				if err := routes.Mount(mux, "", []routes.Route{{Method: http.MethodGet, Path: prefix, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusNoContent)
				})}}); err != nil {
					t.Fatal(err)
				}
				base := strings.ReplaceAll(prefix, "{id:[0-9]+}", "42")
				for path, want := range map[string]int{base: http.StatusNoContent, base + "/users": http.StatusAccepted} {
					recorder := httptest.NewRecorder()
					mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
					if recorder.Code != want {
						t.Fatalf("GET %s: status %d, want %d", path, recorder.Code, want)
					}
				}
			})
		}
	}
}

func TestMountChecksEveryExistingParameterName(t *testing.T) {
	mux := chi.NewRouter()
	mux.Get("/users/{id}/posts", func(http.ResponseWriter, *http.Request) {})
	mux.Get("/users/{name}/profile", func(http.ResponseWriter, *http.Request) {})
	for _, name := range []string{"id", "name"} {
		assertRouteConflict(t, mux, "", []routes.Route{{Method: http.MethodGet, Path: "/users/{" + name + "}/settings", Handler: http.NotFoundHandler()}}, "GET", "parameter")
	}
}

func TestMountValidationLeavesBatchUnregistered(t *testing.T) {
	for _, test := range []struct {
		route routes.Route
		want  string
	}{
		{routes.Route{Method: http.MethodGet, Path: "/nil"}, "routes: route[1] GET /nil: nil handler"},
		{routes.Route{Method: http.MethodGet, Path: "/typed-nil", Handler: (*http.ServeMux)(nil)}, "routes: route[1] GET /typed-nil: nil handler"},
		{routes.Route{Method: http.MethodGet, Path: "/auth", Auth: "invalid", Handler: http.NotFoundHandler()}, `routes: route[1] GET /auth: unsupported auth requirement "invalid"`},
	} {
		t.Run(test.route.Path, func(t *testing.T) {
			mux := chi.NewRouter()
			err := routes.Mount(mux, "", []routes.Route{
				{Method: http.MethodGet, Path: "/new", Handler: http.NotFoundHandler()},
				test.route,
			})
			if err == nil || err.Error() != test.want {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			if len(mux.Routes()) != 0 {
				t.Fatal("validation failure registered part of the batch")
			}
		})
	}
}

func TestMountInvalidSyntaxLeavesBatchUnregistered(t *testing.T) {
	for _, test := range []struct {
		route routes.Route
		want  string
	}{
		{routes.Route{Method: "INVALID", Path: "/method", Handler: http.NotFoundHandler()}, "chi: 'INVALID' http method is not supported."},
		{routes.Route{Method: http.MethodGet, Path: "/files*/tail", Handler: http.NotFoundHandler()}, "chi: wildcard '*' must be the last value in a route. trim trailing text or use a '{param}' instead"},
		{routes.Route{Method: http.MethodGet, Path: "/{id:[}", Handler: http.NotFoundHandler()}, "chi: invalid regexp pattern '^[$' in route param"},
	} {
		t.Run(test.route.Path, func(t *testing.T) {
			mux := chi.NewRouter()
			func() {
				defer func() {
					value := recover()
					message, ok := value.(string)
					if !ok || message != test.want {
						t.Fatalf("panic = %#v, want %q", value, test.want)
					}
				}()
				if err := routes.Mount(mux, "", []routes.Route{
					{Method: http.MethodGet, Path: "/new", Handler: http.NotFoundHandler()},
					test.route,
				}); err != nil {
					t.Fatalf("expected syntax panic, got error: %v", err)
				}
			}()
			if len(mux.Routes()) != 0 {
				t.Fatal("invalid syntax registered part of the batch")
			}
		})
	}
}

func assertRouteConflict(t *testing.T, router chi.Router, prefix string, batch []routes.Route, details ...string) {
	t.Helper()
	defer func() {
		value := recover()
		if value == nil {
			t.Fatal("expected registration conflict panic")
		}
		message, ok := value.(string)
		if !ok || !strings.HasPrefix(message, "routes: route[") {
			t.Fatalf("unexpected panic: %#v", value)
		}
		for _, detail := range details {
			if !strings.Contains(message, detail) {
				t.Errorf("panic %q does not identify %q", message, detail)
			}
		}
	}()
	if err := routes.Mount(router, prefix, batch); err != nil {
		t.Fatalf("expected panic, got error: %v", err)
	}
}
