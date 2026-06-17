package routes_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/Ithildur/EiluneKit/http/routes"

	"github.com/go-chi/chi/v5"
)

func TestBlueprintIncludesChildRoutes(t *testing.T) {
	child := routes.NewBlueprint()
	child.Get(
		"/status",
		"Get status",
		routes.Func(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
		routes.Tags("child"),
		routes.Auth(routes.AuthOptional),
	)

	parent := routes.NewBlueprint()
	parent.Include("/updater", child, routes.IncludeTags("updater"))

	payload, err := parent.ExportJSON()
	if err != nil {
		t.Fatalf("export json: %v", err)
	}

	var exported []struct {
		Method string                 `json:"method"`
		Path   string                 `json:"path"`
		Tags   []string               `json:"tags"`
		Auth   routes.AuthRequirement `json:"auth"`
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
	if got, want := exported[0].Auth, routes.AuthOptional; got != want {
		t.Fatalf("expected auth %q, got %q", want, got)
	}

	r := chi.NewRouter()
	if err := parent.Mount(r); err != nil {
		t.Fatalf("mount: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/updater/status", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
}

func TestFuncReadsDynamicPath(t *testing.T) {
	blueprint := routes.NewBlueprint()
	blueprint.Get(
		"/remotes/{remoteID}",
		"Get remote",
		routes.Func(func(w http.ResponseWriter, r *http.Request, remoteID string) {
			if got, want := remoteID, "origin"; got != want {
				t.Fatalf("expected remoteID %q, got %q", want, got)
			}
			w.WriteHeader(http.StatusNoContent)
		}),
	)

	r := chi.NewRouter()
	if err := blueprint.Mount(r); err != nil {
		t.Fatalf("mount: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/remotes/origin", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
}

func TestFuncReadsRegexpDynamicPath(t *testing.T) {
	blueprint := routes.NewBlueprint()
	blueprint.Get(
		"/remotes/{remoteID:[a-z]{2}[0-9]{3}}",
		"Get remote",
		routes.Func(func(w http.ResponseWriter, r *http.Request, remoteID string) {
			if got, want := remoteID, "ab123"; got != want {
				t.Fatalf("expected remoteID %q, got %q", want, got)
			}
			w.WriteHeader(http.StatusNoContent)
		}),
	)

	r := chi.NewRouter()
	if err := blueprint.Mount(r); err != nil {
		t.Fatalf("mount: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/remotes/ab123", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
}

func TestFuncReadsPrefixedDynamicPath(t *testing.T) {
	blueprint := routes.NewBlueprint()
	blueprint.Get(
		"/remotes/{remoteID}",
		"Get remote",
		routes.Func(func(w http.ResponseWriter, r *http.Request, tenantID, remoteID string) {
			if got, want := tenantID, "acme"; got != want {
				t.Fatalf("expected tenantID %q, got %q", want, got)
			}
			if got, want := remoteID, "origin"; got != want {
				t.Fatalf("expected remoteID %q, got %q", want, got)
			}
			w.WriteHeader(http.StatusNoContent)
		}),
	)

	r := chi.NewRouter()
	if err := blueprint.MountAt(r, "/tenants/{tenantID}"); err != nil {
		t.Fatalf("mount: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/tenants/acme/remotes/origin", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
}

func TestFuncReadsTenDynamicPathParams(t *testing.T) {
	blueprint := routes.NewBlueprint()
	blueprint.Get(
		"/{a}/{b}/{c}/{d}/{e}/{f}/{g}/{h}/{i}/{j}",
		"Get nested resource",
		routes.Func(func(
			w http.ResponseWriter,
			r *http.Request,
			a, b, c, d, e, f, g, h, i, j string,
		) {
			got := []string{a, b, c, d, e, f, g, h, i, j}
			want := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("expected path params %#v, got %#v", want, got)
			}
			w.WriteHeader(http.StatusNoContent)
		}),
	)

	r := chi.NewRouter()
	if err := blueprint.Mount(r); err != nil {
		t.Fatalf("mount: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/a/b/c/d/e/f/g/h/i/j", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
}

func TestFuncRejectsMismatchedDynamicPath(t *testing.T) {
	blueprint := routes.NewBlueprint()
	blueprint.Get(
		"/remotes/{remoteID}",
		"Get remote",
		routes.Func(func(w http.ResponseWriter, r *http.Request, tenantID, remoteID string) {}),
	)

	err := blueprint.Mount(chi.NewRouter())
	if err == nil {
		t.Fatal("expected path params mismatch error")
	}
}

func TestFuncRejectsDuplicateDynamicPathNames(t *testing.T) {
	blueprint := routes.NewBlueprint()
	blueprint.Get(
		"/remotes/{id}",
		"Get remote",
		routes.Func(func(w http.ResponseWriter, r *http.Request, tenantID, remoteID string) {}),
	)

	err := blueprint.MountAt(chi.NewRouter(), "/tenants/{id}")
	if err == nil {
		t.Fatal("expected duplicate path param error")
	}
	if !strings.Contains(err.Error(), `duplicate path param "id"`) {
		t.Fatalf("expected duplicate path param error, got %v", err)
	}
}

func TestExportOpenAPI(t *testing.T) {
	payload, err := routes.ExportOpenAPI([]routes.Route{
		{
			Method:  http.MethodGet,
			Path:    "users/{id}",
			Summary: "Get user",
			Tags:    []string{"users"},
			Auth:    routes.AuthRequired,
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
		},
		{
			Method:  http.MethodPost,
			Path:    "/sessions",
			Summary: "Create session",
			Auth:    routes.AuthOptional,
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
		},
	}, routes.OpenAPIOptions{
		Title:   "Admin API",
		Version: "1.2.3",
	})
	if err != nil {
		t.Fatalf("export openapi: %v", err)
	}

	var doc struct {
		OpenAPI string `json:"openapi"`
		Info    struct {
			Title   string `json:"title"`
			Version string `json:"version"`
		} `json:"info"`
		Paths map[string]map[string]struct {
			Summary  string                `json:"summary"`
			Tags     []string              `json:"tags"`
			Security []map[string][]string `json:"security"`
		} `json:"paths"`
		Components struct {
			SecuritySchemes map[string]struct {
				Type   string `json:"type"`
				Scheme string `json:"scheme"`
			} `json:"securitySchemes"`
		} `json:"components"`
	}
	if err := json.Unmarshal(payload, &doc); err != nil {
		t.Fatalf("unmarshal openapi: %v", err)
	}
	if got, want := doc.OpenAPI, "3.0.3"; got != want {
		t.Fatalf("expected openapi %q, got %q", want, got)
	}
	if got, want := doc.Info.Title, "Admin API"; got != want {
		t.Fatalf("expected title %q, got %q", want, got)
	}
	if got, want := doc.Info.Version, "1.2.3"; got != want {
		t.Fatalf("expected version %q, got %q", want, got)
	}
	getUser := doc.Paths["/users/{id}"]["get"]
	if got, want := getUser.Summary, "Get user"; got != want {
		t.Fatalf("expected summary %q, got %q", want, got)
	}
	if !reflect.DeepEqual(getUser.Tags, []string{"users"}) {
		t.Fatalf("expected users tag, got %#v", getUser.Tags)
	}
	if got := getUser.Security[0]["BearerAuth"]; got == nil {
		t.Fatalf("expected BearerAuth security, got %#v", getUser.Security)
	}
	createSession := doc.Paths["/sessions"]["post"]
	if got, want := len(createSession.Security), 2; got != want {
		t.Fatalf("expected optional auth security alternatives, got %#v", createSession.Security)
	}
	scheme := doc.Components.SecuritySchemes["BearerAuth"]
	if scheme.Type != "http" || scheme.Scheme != "bearer" {
		t.Fatalf("unexpected bearer security scheme: %#v", scheme)
	}
}

func TestBlueprintDefaults(t *testing.T) {
	var calls []string
	defaultMW := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls = append(calls, "default")
			next.ServeHTTP(w, r.WithContext(routes.WithAuthenticated(r.Context())))
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
		routes.DefaultAuth(routes.AuthRequired),
		routes.DefaultMiddleware(defaultMW),
	)
	blueprint.Get(
		"/users",
		"List users",
		routes.Func(func(w http.ResponseWriter, r *http.Request) {
			calls = append(calls, "handler")
			w.WriteHeader(http.StatusNoContent)
		}),
		routes.Tags("users"),
		routes.Use(routeMW),
	)
	blueprint.Get(
		"/public",
		"",
		routes.Func(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
		routes.Auth(routes.AuthPublic),
	)

	exportedRoutes := blueprint.Routes()
	if got, want := len(exportedRoutes), 2; got != want {
		t.Fatalf("expected %d routes, got %d", want, got)
	}
	if !reflect.DeepEqual(exportedRoutes[0].Tags, []string{"admin", "users"}) {
		t.Fatalf("expected default and route tags, got %#v", exportedRoutes[0].Tags)
	}
	if got, want := exportedRoutes[0].Auth, routes.AuthRequired; got != want {
		t.Fatalf("expected default auth %q, got %q", want, got)
	}
	if got, want := exportedRoutes[1].Auth, routes.AuthPublic; got != want {
		t.Fatalf("expected explicit auth %q, got %q", want, got)
	}

	r := chi.NewRouter()
	if err := blueprint.MountAt(r, "/api"); err != nil {
		t.Fatalf("mount: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
	if !reflect.DeepEqual(calls, []string{"default", "route", "handler"}) {
		t.Fatalf("unexpected call order: %#v", calls)
	}
}

func TestBlueprintIncludeMiddlewarePrependsChildRoutes(t *testing.T) {
	var calls []string
	includeMW := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls = append(calls, "include")
			next.ServeHTTP(w, r)
		})
	}
	routeMW := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls = append(calls, "route")
			next.ServeHTTP(w, r)
		})
	}

	child := routes.NewBlueprint()
	child.Get("/public", "", routes.Func(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, "handler")
		w.WriteHeader(http.StatusNoContent)
	}), routes.Use(routeMW))

	parent := routes.NewBlueprint()
	parent.Include("/child", child, routes.IncludeMiddleware(includeMW))

	exportedRoutes := parent.Routes()
	if got, want := len(exportedRoutes), 1; got != want {
		t.Fatalf("expected %d route, got %d", want, got)
	}
	if got, want := exportedRoutes[0].Path, "/child/public"; got != want {
		t.Fatalf("expected included path %q, got %q", want, got)
	}

	r := chi.NewRouter()
	if err := parent.Mount(r); err != nil {
		t.Fatalf("mount: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/child/public", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
	if !reflect.DeepEqual(calls, []string{"include", "route", "handler"}) {
		t.Fatalf("unexpected call order: %#v", calls)
	}
}

func TestBlueprintIncludeAuthOverridesChildRoutes(t *testing.T) {
	child := routes.NewBlueprint()
	child.Get("/public", "", routes.Func(func(w http.ResponseWriter, r *http.Request) {}), routes.Auth(routes.AuthPublic))

	parent := routes.NewBlueprint()
	parent.Include("/child", child, routes.IncludeAuth(routes.AuthRequired))

	exportedRoutes := parent.Routes()
	if got, want := len(exportedRoutes), 1; got != want {
		t.Fatalf("expected %d route, got %d", want, got)
	}
	if got, want := exportedRoutes[0].Auth, routes.AuthRequired; got != want {
		t.Fatalf("expected include auth %q, got %q", want, got)
	}
}

func TestMountAppliesRouteMiddleware(t *testing.T) {
	var calls []string
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
			Handler:    handler,
			Middleware: []routes.Middleware{routeMW},
		},
	})
	if err != nil {
		t.Fatalf("mount: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
	if !reflect.DeepEqual(calls, []string{"route", "handler"}) {
		t.Fatalf("unexpected call order: %#v", calls)
	}
}

func TestMountRequiresAuthenticatedContext(t *testing.T) {
	called := false
	r := chi.NewRouter()
	err := routes.Mount(r, "/api", []routes.Route{
		{
			Method: "get",
			Path:   "users",
			Auth:   routes.AuthRequired,
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusNoContent)
			}),
		},
	})
	if err != nil {
		t.Fatalf("mount: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
	if called {
		t.Fatal("handler should not be called")
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

func TestExportJSONSortsRoutesAndTags(t *testing.T) {
	payload, err := routes.ExportJSON([]routes.Route{
		{
			Method:  "post",
			Path:    "/b",
			Summary: "create",
			Tags:    []string{"z", "a"},
			Auth:    routes.AuthRequired,
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
		Method string                 `json:"method"`
		Path   string                 `json:"path"`
		Tags   []string               `json:"tags"`
		Auth   routes.AuthRequirement `json:"auth"`
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
	if got, want := exported[0].Auth, routes.AuthPublic; got != want {
		t.Fatalf("expected default auth %q, got %q", want, got)
	}
	if got, want := exported[1].Auth, routes.AuthRequired; got != want {
		t.Fatalf("expected exported auth %q, got %q", want, got)
	}
	if !reflect.DeepEqual(exported[1].Tags, []string{"a", "z"}) {
		t.Fatalf("expected sorted tags, got %#v", exported[1].Tags)
	}
}

func TestExportMarkdownIncludesAuthRequirement(t *testing.T) {
	markdown, err := routes.ExportMarkdown([]routes.Route{
		{Method: "get", Path: "/public"},
		{Method: "post", Path: "/secure", Auth: routes.AuthRequired},
	})
	if err != nil {
		t.Fatalf("export markdown: %v", err)
	}

	for _, want := range []string{
		"| Method | Path | Summary | Auth | Tags |",
		"| GET | /public |  | public |  |",
		"| POST | /secure |  | required |  |",
	} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("expected markdown to contain %q, got:\n%s", want, markdown)
		}
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

func TestHandlerRejectsNilHandlers(t *testing.T) {
	mustPanic(t, func() {
		routes.Handler(nil)
	})

	var fn func(http.ResponseWriter, *http.Request)
	mustPanic(t, func() {
		routes.Func(fn)
	})

	var paramFn func(http.ResponseWriter, *http.Request, string)
	mustPanic(t, func() {
		routes.Func(paramFn)
	})

	var h *typedNilHandler
	mustPanic(t, func() {
		routes.Handler(h)
	})
}

func TestMountRejectsTypedNilHandler(t *testing.T) {
	var h *typedNilHandler

	err := routes.Mount(chi.NewRouter(), "", []routes.Route{
		{
			Method:  http.MethodGet,
			Path:    "/health",
			Handler: h,
		},
	})
	if err == nil {
		t.Fatal("expected nil handler error")
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

type typedNilHandler struct{}

func (*typedNilHandler) ServeHTTP(http.ResponseWriter, *http.Request) {}
