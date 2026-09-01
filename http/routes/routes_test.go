package routes_test

import (
	"encoding/json/v2"
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
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		},
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

func TestBlueprintRoutesAtNormalizesPathBeforePrefix(t *testing.T) {
	blueprint := routes.NewBlueprint()
	blueprint.Add(routes.Route{Path: " users "})

	for _, test := range []struct {
		prefix string
		want   string
	}{
		{prefix: " /api/ ", want: "/api/users"},
		{prefix: "/", want: "/users"},
	} {
		routeList := blueprint.RoutesAt(test.prefix)
		if got := routeList[0].Path; got != test.want {
			t.Fatalf("prefix %q: expected final path %q, got %q", test.prefix, test.want, got)
		}
	}

	routeList := blueprint.RoutesAt("/tenants/{tenantID}")
	params := routeList[0].Parameters
	if len(params) != 1 || params[0].Name != "tenantID" || params[0].In != routes.ParameterPath || !params[0].Required {
		t.Fatalf("unexpected dynamic prefix parameters: %#v", params)
	}
}

func TestBlueprintHandlerReadsDynamicPath(t *testing.T) {
	blueprint := routes.NewBlueprint()
	blueprint.Get(
		"/remotes/{remoteID}",
		"Get remote",
		func(w http.ResponseWriter, r *http.Request, remoteID string) {
			if got, want := remoteID, "origin"; got != want {
				t.Fatalf("expected remoteID %q, got %q", want, got)
			}
			w.WriteHeader(http.StatusNoContent)
		},
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

func TestBlueprintHandlerReadsRegexpDynamicPath(t *testing.T) {
	blueprint := routes.NewBlueprint()
	blueprint.Get(
		"/remotes/{remoteID:[a-z]{2}[0-9]{3}}",
		"Get remote",
		func(w http.ResponseWriter, r *http.Request, remoteID string) {
			if got, want := remoteID, "ab123"; got != want {
				t.Fatalf("expected remoteID %q, got %q", want, got)
			}
			w.WriteHeader(http.StatusNoContent)
		},
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

func TestBlueprintHandlerReadsSimpleRegexpDynamicPath(t *testing.T) {
	blueprint := routes.NewBlueprint()
	blueprint.Get(
		"/users/{id:[0-9]+}",
		"Get user",
		func(w http.ResponseWriter, r *http.Request, id string) {
			if got, want := id, "42"; got != want {
				t.Fatalf("expected id %q, got %q", want, got)
			}
			w.WriteHeader(http.StatusNoContent)
		},
	)

	r := chi.NewRouter()
	if err := blueprint.Mount(r); err != nil {
		t.Fatalf("mount: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
}

func TestBlueprintHandlerReadsWildcardDynamicPath(t *testing.T) {
	blueprint := routes.NewBlueprint()
	blueprint.Get(
		"/files/*",
		"Get file",
		func(w http.ResponseWriter, r *http.Request, path string) {
			if got, want := path, "a/b/c.txt"; got != want {
				t.Fatalf("expected path %q, got %q", want, got)
			}
			w.WriteHeader(http.StatusNoContent)
		},
	)

	r := chi.NewRouter()
	if err := blueprint.Mount(r); err != nil {
		t.Fatalf("mount: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/files/a/b/c.txt", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
}

func TestBlueprintHandlerReadsPrefixedDynamicPath(t *testing.T) {
	blueprint := routes.NewBlueprint()
	blueprint.Get(
		"/remotes/{remoteID}",
		"Get remote",
		func(w http.ResponseWriter, r *http.Request, tenantID, remoteID string) {
			if got, want := tenantID, "acme"; got != want {
				t.Fatalf("expected tenantID %q, got %q", want, got)
			}
			if got, want := remoteID, "origin"; got != want {
				t.Fatalf("expected remoteID %q, got %q", want, got)
			}
			w.WriteHeader(http.StatusNoContent)
		},
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

func TestBlueprintHandlerReadsTenDynamicPathParams(t *testing.T) {
	blueprint := routes.NewBlueprint()
	blueprint.Get(
		"/{a}/{b}/{c}/{d}/{e}/{f}/{g}/{h}/{i}/{j}",
		"Get nested resource",
		func(
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
		},
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

func TestBlueprintHandlerRejectsMismatchedDynamicPath(t *testing.T) {
	blueprint := routes.NewBlueprint()
	blueprint.Get(
		"/remotes/{remoteID}",
		"Get remote",
		func(w http.ResponseWriter, r *http.Request, tenantID, remoteID string) {},
	)

	err := blueprint.Mount(chi.NewRouter())
	if err == nil {
		t.Fatal("expected path params mismatch error")
	}
}

func TestBlueprintHandlerRejectsDuplicateDynamicPathNames(t *testing.T) {
	blueprint := routes.NewBlueprint()
	blueprint.Get(
		"/remotes/{id}",
		"Get remote",
		func(w http.ResponseWriter, r *http.Request, tenantID, remoteID string) {},
	)

	err := blueprint.MountAt(chi.NewRouter(), "/tenants/{id}")
	if err == nil {
		t.Fatal("expected duplicate path param error")
	}
	if !strings.Contains(err.Error(), `duplicate path param "id"`) {
		t.Fatalf("expected duplicate path param error, got %v", err)
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
		func(w http.ResponseWriter, r *http.Request) {
			calls = append(calls, "handler")
			w.WriteHeader(http.StatusNoContent)
		},
		routes.Tags("users"),
		routes.Use(routeMW),
	)
	blueprint.Get(
		"/public",
		"",
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		},
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
	child.Get("/public", "", func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, "handler")
		w.WriteHeader(http.StatusNoContent)
	}, routes.Use(routeMW))

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
	child.Get("/public", "", func(w http.ResponseWriter, r *http.Request) {}, routes.Auth(routes.AuthPublic))

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

func TestMountRejectsUnknownAuthRequirement(t *testing.T) {
	err := routes.Mount(chi.NewRouter(), "", []routes.Route{
		{
			Method:  http.MethodGet,
			Path:    "/private",
			Auth:    routes.AuthRequirement("require"),
			Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		},
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported auth requirement") {
		t.Fatalf("expected unsupported auth requirement error, got %v", err)
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

func TestBlueprintRejectsNilHandlerFunctions(t *testing.T) {
	b := routes.NewBlueprint()
	var fn func(http.ResponseWriter, *http.Request)
	mustPanic(t, func() {
		b.Get("/", "", fn)
	})

	var paramFn func(http.ResponseWriter, *http.Request, string)
	mustPanic(t, func() {
		b.Get("/{id}", "", paramFn)
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

func TestRouteCloneOwnsContractMetadata(t *testing.T) {
	original := routes.Route{
		Parameters: []routes.Parameter{{Name: "id"}},
		RequestBody: &routes.RequestBody{
			Content: routes.Content{"application/json": routes.SchemaOf[string]("Request")},
		},
		Responses: map[string]routes.Response{
			"200": {
				Content: routes.Content{"application/json": routes.SchemaOf[string]("Response")},
			},
		},
		Security: []routes.SecurityRequirement{
			{{Name: "BearerAuth"}},
		},
	}

	cloned := original.Clone()
	cloned.Parameters[0].Name = "other"
	cloned.RequestBody.Content["application/json"] = routes.SchemaOf[int]("OtherRequest")
	response := cloned.Responses["200"]
	response.Content["application/json"] = routes.SchemaOf[int]("OtherResponse")
	cloned.Responses["200"] = response
	cloned.Security[0][0].Name = "OtherAuth"

	if got := original.Parameters[0].Name; got != "id" {
		t.Fatalf("parameter aliasing changed original to %q", got)
	}
	if got := original.RequestBody.Content["application/json"].Name; got != "Request" {
		t.Fatalf("request body aliasing changed original to %q", got)
	}
	if got := original.Responses["200"].Content["application/json"].Name; got != "Response" {
		t.Fatalf("response aliasing changed original to %q", got)
	}
	if got := original.Security[0][0].Name; got != "BearerAuth" {
		t.Fatalf("security requirement aliasing changed original to %q", got)
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
