package routes_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/Ithildur/EiluneKit/http/routes"

	"github.com/go-chi/chi/v5"
)

func TestBlueprintIncludesChildRoutes(t *testing.T) {
	child := routes.NewBlueprint()
	child.Get(
		"/status",
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
		routes.Summary("Get status"),
		routes.Tags("child"),
		routes.Auth(routes.AuthBearerOptional),
	)

	parent := routes.NewBlueprint()
	parent.Include("/updater", child, routes.IncludeTags("updater"))

	payload, err := parent.ExportJSON()
	if err != nil {
		t.Fatalf("export json: %v", err)
	}

	var exported []struct {
		Method string            `json:"method"`
		Path   string            `json:"path"`
		Tags   []string          `json:"tags"`
		Auth   routes.AuthPolicy `json:"auth"`
	}
	if err := json.Unmarshal(payload, &exported); err != nil {
		t.Fatalf("unmarshal export: %v", err)
	}
	if got, want := len(exported), 1; got != want {
		t.Fatalf("expected %d exported route, got %d", want, got)
	}
	if got, want := exported[0].Method, http.MethodGet; got != want {
		t.Fatalf("expected method %q, got %q", want, got)
	}
	if got, want := exported[0].Path, "/updater/status"; got != want {
		t.Fatalf("expected included path %q, got %q", want, got)
	}
	if !reflect.DeepEqual(exported[0].Tags, []string{"child", "updater"}) {
		t.Fatalf("expected include tags, got %#v", exported[0].Tags)
	}
	if got, want := exported[0].Auth, routes.AuthBearerOptional; got != want {
		t.Fatalf("expected auth %q, got %q", want, got)
	}

	r := chi.NewRouter()
	if err := parent.Mount(r, routes.WithAuth(routes.AuthResolver{
		routes.AuthBearerOptional: func(next http.Handler) http.Handler { return next },
	})); err != nil {
		t.Fatalf("mount: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/updater/status", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
}

func TestBlueprintDefaults(t *testing.T) {
	var calls []string
	authMW := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls = append(calls, "auth")
			next.ServeHTTP(w, r)
		})
	}
	defaultMW := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls = append(calls, "default")
			next.ServeHTTP(w, r)
		})
	}
	routeMW := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls = append(calls, "route")
			next.ServeHTTP(w, r)
		})
	}

	blueprint := routes.NewBlueprint(
		routes.DefaultTags("admin"),
		routes.DefaultAuth(routes.AuthBearerRequired),
		routes.DefaultMiddleware(defaultMW),
	)
	blueprint.Get(
		"/users",
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls = append(calls, "handler")
			w.WriteHeader(http.StatusNoContent)
		}),
		routes.Summary("List users"),
		routes.Tags("users"),
		routes.Use(routeMW),
	)
	blueprint.Get(
		"/public",
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
		routes.Auth(routes.AuthNone),
	)

	exportedRoutes := blueprint.Routes()
	if got, want := len(exportedRoutes), 2; got != want {
		t.Fatalf("expected %d routes, got %d", want, got)
	}
	if !reflect.DeepEqual(exportedRoutes[0].Tags, []string{"admin", "users"}) {
		t.Fatalf("expected default and route tags, got %#v", exportedRoutes[0].Tags)
	}
	if got, want := exportedRoutes[0].Auth, routes.AuthBearerRequired; got != want {
		t.Fatalf("expected inherited auth %q, got %q", want, got)
	}
	if got, want := exportedRoutes[1].Auth, routes.AuthNone; got != want {
		t.Fatalf("expected explicit auth override %q, got %q", want, got)
	}

	r := chi.NewRouter()
	if err := blueprint.MountAt(r, "/api", routes.WithAuth(routes.AuthResolver{
		routes.AuthBearerRequired: authMW,
	})); err != nil {
		t.Fatalf("mount: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
	if !reflect.DeepEqual(calls, []string{"auth", "default", "route", "handler"}) {
		t.Fatalf("unexpected call order: %#v", calls)
	}
}

func TestBlueprintIncludeAuthOverridesChildRoutes(t *testing.T) {
	child := routes.NewBlueprint()
	child.Get("/public", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), routes.Auth(routes.AuthNone))

	parent := routes.NewBlueprint()
	parent.Include("/child", child, routes.IncludeAuth(routes.AuthBearerRequired))

	exportedRoutes := parent.Routes()
	if got, want := len(exportedRoutes), 1; got != want {
		t.Fatalf("expected %d route, got %d", want, got)
	}
	if got, want := exportedRoutes[0].Path, "/child/public"; got != want {
		t.Fatalf("expected included path %q, got %q", want, got)
	}
	if got, want := exportedRoutes[0].Auth, routes.AuthBearerRequired; got != want {
		t.Fatalf("expected include auth %q, got %q", want, got)
	}
}

func TestMountAppliesAuthAndRouteMiddleware(t *testing.T) {
	var calls []string
	authMW := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls = append(calls, "auth")
			next.ServeHTTP(w, r)
		})
	}
	routeMW := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls = append(calls, "route")
			next.ServeHTTP(w, r)
		})
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, "handler")
		w.WriteHeader(http.StatusNoContent)
	})

	r := chi.NewRouter()
	err := routes.Mount(r, "/api", []routes.Route{
		{
			Method:     "get",
			Path:       "users",
			Auth:       routes.AuthBearerRequired,
			Handler:    handler,
			Middleware: []routes.Middleware{routeMW},
		},
	}, routes.WithAuth(routes.AuthResolver{
		routes.AuthBearerRequired: authMW,
	}))
	if err != nil {
		t.Fatalf("mount: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
	if !reflect.DeepEqual(calls, []string{"auth", "route", "handler"}) {
		t.Fatalf("unexpected call order: %#v", calls)
	}
}

func TestMountRejectsDuplicateNormalizedRoutes(t *testing.T) {
	r := chi.NewRouter()
	err := routes.Mount(r, "", []routes.Route{
		{Method: "get", Path: "users", Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})},
		{Method: "GET", Path: "/users", Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})},
	})
	if err == nil {
		t.Fatal("expected duplicate route error")
	}
}

func TestMountRejectsProtectedRouteWithoutAuthResolver(t *testing.T) {
	r := chi.NewRouter()
	err := routes.Mount(r, "", []routes.Route{
		{
			Method:  "GET",
			Path:    "/protected",
			Auth:    routes.AuthBearerRequired,
			Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		},
	})
	if err == nil {
		t.Fatal("expected auth resolver error")
	}
}

func TestExportJSONSortsRoutesAndTags(t *testing.T) {
	payload, err := routes.ExportJSON([]routes.Route{
		{
			Method:  "post",
			Path:    "/b",
			Summary: "create",
			Tags:    []string{"z", "a"},
			Auth:    routes.AuthBearerRequired,
		},
		{
			Method:  "get",
			Path:    "a",
			Summary: "list",
		},
	})
	if err != nil {
		t.Fatalf("export json: %v", err)
	}

	var exported []struct {
		Method string            `json:"method"`
		Path   string            `json:"path"`
		Tags   []string          `json:"tags"`
		Auth   routes.AuthPolicy `json:"auth"`
	}
	if err := json.Unmarshal(payload, &exported); err != nil {
		t.Fatalf("unmarshal export: %v", err)
	}
	if got, want := len(exported), 2; got != want {
		t.Fatalf("expected %d exported routes, got %d", want, got)
	}
	if got, want := exported[0].Path, "/a"; got != want {
		t.Fatalf("expected first path %q, got %q", want, got)
	}
	if got, want := exported[0].Auth, routes.AuthNone; got != want {
		t.Fatalf("expected default auth %q, got %q", want, got)
	}
	if !reflect.DeepEqual(exported[1].Tags, []string{"a", "z"}) {
		t.Fatalf("expected sorted tags, got %#v", exported[1].Tags)
	}
}

func TestBlueprintAndRouterRejectNilReceiver(t *testing.T) {
	t.Run("blueprint", func(t *testing.T) {
		var b *routes.Blueprint
		mustPanic(t, func() {
			b.Add(routes.Route{Method: http.MethodGet, Path: "/health"})
		})
	})

	t.Run("router", func(t *testing.T) {
		var r *routes.Router
		mustPanic(t, func() {
			r.Include("/api", nil)
		})
	})
}

func TestMountRejectsNilChiRouter(t *testing.T) {
	err := routes.Mount(nil, "", []routes.Route{
		{
			Method:  http.MethodGet,
			Path:    "/health",
			Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		},
	})
	if err == nil {
		t.Fatal("expected nil chi.Router error")
	}
}

func mustPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}
