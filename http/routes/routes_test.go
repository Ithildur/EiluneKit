package routes_test

import (
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/Ithildur/EiluneKit/http/middleware"
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
	parent.Include("updater", child, routes.IncludeTags("updater"))

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

func TestBlueprintRoutesAtComposesPaths(t *testing.T) {
	blueprint := routes.NewBlueprint()
	blueprint.Add(routes.Route{Path: "/users"})

	for _, test := range []struct {
		prefix string
		want   string
	}{
		{prefix: "api", want: "/api/users"},
		{prefix: "", want: "/users"},
	} {
		routeList := blueprint.RoutesAt(test.prefix)
		if got := routeList[0].Path; got != test.want {
			t.Fatalf("prefix %q: expected final path %q, got %q", test.prefix, test.want, got)
		}
	}

	routeList := blueprint.RoutesAt("tenants/{tenantID}")
	params := routeList[0].Parameters
	if len(params) != 1 || params[0].Name != "tenantID" || params[0].In != routes.ParameterPath || !params[0].Required {
		t.Fatalf("unexpected dynamic prefix parameters: %#v", params)
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
	if err := blueprint.MountAt(r, "api"); err != nil {
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
	parent.Include("child", child, routes.IncludeMiddleware(includeMW))

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
	parent.Include("child", child, routes.IncludeAuth(routes.AuthRequired))

	exportedRoutes := parent.Routes()
	if got, want := len(exportedRoutes), 1; got != want {
		t.Fatalf("expected %d route, got %d", want, got)
	}
	if got, want := exportedRoutes[0].Auth, routes.AuthRequired; got != want {
		t.Fatalf("expected include auth %q, got %q", want, got)
	}
}

func TestMountRequiresAuthenticatedContext(t *testing.T) {
	called := false
	r := chi.NewRouter()
	err := routes.Mount(r, "api", []routes.Route{
		{
			Method: "get",
			Path:   "/users",
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

func TestMountUsesFinalPaths(t *testing.T) {
	b := routes.NewBlueprint()
	b.Get("", "", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusAccepted) })
	b.Get("/", "", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	b.Get("/items/{id}", "", func(w http.ResponseWriter, r *http.Request, id string) { w.WriteHeader(http.StatusNoContent) })
	for _, mount := range []string{"prefix", "expanded", "include", "router"} {
		t.Run(mount, func(t *testing.T) {
			mux := chi.NewRouter()
			mux.MethodNotAllowed(middleware.MethodNotAllowedResponder(mux))
			var err error
			switch mount {
			case "prefix":
				err = b.MountAt(mux, "api")
			case "expanded":
				err = routes.Mount(mux, "", b.RoutesAt("api"))
			case "include":
				parent := routes.NewBlueprint()
				parent.Include("", b)
				err = parent.MountAt(mux, "api")
			case "router":
				parent := routes.NewRouter()
				parent.Include("", b.Routes())
				err = parent.Mount(mux, "api")
			}
			if err != nil {
				t.Fatal(err)
			}
			// Separate mounting calls may share a prefix.
			// 独立的挂载调用可以共享前缀。
			if err := routes.Mount(mux, "api", []routes.Route{{Method: http.MethodGet, Path: "/other", Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })}}); err != nil {
				t.Fatal(err)
			}
			for _, test := range []struct {
				method, path string
				status       int
			}{
				{http.MethodGet, "/api", http.StatusAccepted},
				{http.MethodGet, "/api/", http.StatusNoContent},
				{http.MethodGet, "/api/items/42", http.StatusNoContent},
				{http.MethodGet, "/api/other", http.StatusNoContent},
				{http.MethodGet, "/api/missing", http.StatusNotFound},
				{http.MethodPost, "/api/items/42", http.StatusMethodNotAllowed},
				{http.MethodHead, "/api/items/42", http.StatusMethodNotAllowed},
			} {
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, httptest.NewRequest(test.method, test.path, nil))
				if w.Code != test.status {
					t.Fatalf("%s %s: %d; want %d", test.method, test.path, w.Code, test.status)
				}
				if test.status == http.StatusMethodNotAllowed && strings.Join(w.Header().Values("Allow"), ",") != "GET" {
					t.Fatalf("unexpected Allow: %v", w.Header().Values("Allow"))
				}
			}
		})
	}
}

func TestMountNormalizesEmptyFinalPath(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	for _, path := range []string{"", "/"} {
		mux := chi.NewRouter()
		if err := routes.Mount(mux, "", []routes.Route{{Method: http.MethodGet, Path: path, Handler: handler}}); err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		if w.Code != http.StatusNoContent {
			t.Fatalf("path %q: status %d", path, w.Code)
		}
	}
	mustPanic(t, func() {
		_ = routes.Mount(chi.NewRouter(), "", []routes.Route{
			{Method: http.MethodGet, Path: "", Handler: handler},
			{Method: http.MethodGet, Path: "/", Handler: handler},
		})
	})
}

func TestBlueprintIncludeOwnsRoutes(t *testing.T) {
	child := routes.NewBlueprint()
	child.Get("/items", "", func(http.ResponseWriter, *http.Request) {}, routes.Tags("child"), routes.Auth(routes.AuthOptional))
	parent := routes.NewBlueprint(routes.DefaultTags("parent"))
	parent.Include("first", child, routes.IncludeTags("included"), routes.IncludeAuth(routes.AuthRequired))
	parent.Include("second", child)
	got := parent.Routes()
	if got[0].Path != "/first/items" || got[0].Auth != routes.AuthRequired || !reflect.DeepEqual(got[0].Tags, []string{"parent", "child", "included"}) {
		t.Fatalf("unexpected first route: %#v", got[0])
	}
	if got[1].Path != "/second/items" || got[1].Auth != routes.AuthOptional || !reflect.DeepEqual(got[1].Tags, []string{"parent", "child"}) {
		t.Fatalf("unexpected second route: %#v", got[1])
	}
	if original := child.Routes()[0]; original.Path != "/items" || original.Auth != routes.AuthOptional || !reflect.DeepEqual(original.Tags, []string{"child"}) {
		t.Fatalf("child changed: %#v", original)
	}
	parent.Include("copy", parent)
	if len(parent.Routes()) != 4 {
		t.Fatal("self-include did not snapshot existing routes")
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
			Path:    "/a",
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
