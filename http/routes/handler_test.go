package routes_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ithildur/EiluneKit/http/routes"
	"github.com/go-chi/chi/v5"
)

func TestBlueprintHandlerSignatures(t *testing.T) {
	type handler func(http.ResponseWriter, *http.Request)
	type paramsHandler func(http.ResponseWriter, *http.Request, string, string, string, string, string, string, string, string, string, string, string, string, string, string, string)
	b := routes.NewBlueprint()
	b.Get("/status", "", handler(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "ready") }))
	b.Get("/{a}/{b}/{c}/{d}/{e}/{f}/{g}/{h}/{i}/{j}", "",
		func(w http.ResponseWriter, r *http.Request, a, b, c, d, e, f, g, h, i, j string) {
			_, _ = io.WriteString(w, strings.Join([]string{a, b, c, d, e, f, g, h, i, j}, "/"))
		})
	b.Get("/{a}/{b}/{c}/{d}/{e}/{f}/{g}/{h}/{i}/{j}/{k}/{l}/{m}/{n}/{o}", "",
		paramsHandler(func(w http.ResponseWriter, r *http.Request, a, b, c, d, e, f, g, h, i, j, k, l, m, n, o string) {
			_, _ = io.WriteString(w, strings.Join([]string{a, b, c, d, e, f, g, h, i, j, k, l, m, n, o}, "/"))
		}))
	mux := chi.NewRouter()
	if err := b.Mount(mux); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ path, want string }{
		{"/status", "ready"},
		{"/a/b/c/d/e/f/g/h/i/j", "a/b/c/d/e/f/g/h/i/j"},
		{"/a/b/c/d/e/f/g/h/i/j/k/l/m/n/o", "a/b/c/d/e/f/g/h/i/j/k/l/m/n/o"},
	} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, test.path, nil))
		if w.Code != http.StatusOK || w.Body.String() != test.want {
			t.Fatalf("%s: %d %q", test.path, w.Code, w.Body.String())
		}
	}
}

func TestBlueprintRejectsNilHandlers(t *testing.T) {
	type handler func(http.ResponseWriter, *http.Request)
	type paramsHandler func(http.ResponseWriter, *http.Request, string)
	b := routes.NewBlueprint()
	for _, test := range []struct {
		name     string
		register func()
	}{
		{"plain", func() { b.Get("/", "", (func(http.ResponseWriter, *http.Request))(nil)) }},
		{"named", func() { b.Get("/", "", handler(nil)) }},
		{"parameterized", func() { b.Get("/{id}", "", (func(http.ResponseWriter, *http.Request, string))(nil)) }},
		{"named parameterized", func() { b.Get("/{id}", "", paramsHandler(nil)) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				value := recover()
				message, ok := value.(string)
				if !ok || message != "routes: nil handler function" {
					t.Fatalf("panic = %#v, want routes: nil handler function", value)
				}
			}()
			test.register()
		})
	}
}

func TestBlueprintRejectsInvalidBindings(t *testing.T) {
	b := routes.NewBlueprint()
	b.Get("/remotes/{id}", "", func(http.ResponseWriter, *http.Request, string, string) {})
	for _, test := range []struct{ prefix, want string }{
		{"", "handler expects 2 path params, route has 1"},
		{"/tenants/{id}", "duplicate path param \"id\""},
	} {
		t.Run(test.prefix, func(t *testing.T) {
			err := b.MountAt(chi.NewRouter(), test.prefix)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestBlueprintBindingsAreIndependent(t *testing.T) {
	b := routes.NewBlueprint()
	b.Get("/{item}", "", func(w http.ResponseWriter, r *http.Request, owner, item string) {
		_, _ = io.WriteString(w, owner+"/"+item)
	})
	mux := chi.NewRouter()
	for _, prefix := range []string{"/tenants/{tenant}", "/users/{user}"} {
		if err := b.MountAt(mux, prefix); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct{ path, want string }{
		{"/tenants/acme/first", "acme/first"},
		{"/users/alice/second", "alice/second"},
	} {
		t.Run(test.path, func(t *testing.T) {
			t.Parallel()
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, test.path, nil))
			if w.Code != http.StatusOK || w.Body.String() != test.want {
				t.Fatalf("%d %q", w.Code, w.Body.String())
			}
		})
	}
}

func TestBlueprintPathValues(t *testing.T) {
	for _, test := range []struct {
		name, pattern, target, value, want string
		override                           bool
	}{
		{name: "parameter", pattern: "/remotes/{id}", target: "/remotes/origin", want: "origin"},
		{name: "regex", pattern: "/remotes/{id:[a-z]{2}[0-9]{3}}", target: "/remotes/ab123", want: "ab123"},
		{name: "encoded", pattern: "/{id}", target: "/a%2Fb", want: "a%2Fb"},
		{name: "wildcard empty", pattern: "/files/*", target: "/files/", want: ""},
		{name: "wildcard path", pattern: "/files/*", target: "/files/a/b/c.txt", want: "a/b/c.txt"},
		{name: "suffix wildcard empty", pattern: "/files*", target: "/files", want: ""},
		{name: "suffix wildcard path", pattern: "/files*", target: "/files/readme", want: "/readme"},
		{name: "suffix wildcard extension", pattern: "/files*", target: "/files.txt", want: ".txt"},
		{name: "override", pattern: "/{id}", target: "/original", value: "changed", want: "changed", override: true},
		{name: "clear", pattern: "/{id}", target: "/original", want: "", override: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			b := routes.NewBlueprint()
			b.Get(test.pattern, "", func(w http.ResponseWriter, r *http.Request, value string) { _, _ = io.WriteString(w, value) },
				routes.Use(func(next http.Handler) http.Handler {
					return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if test.override {
							r.SetPathValue("id", test.value)
						}
						next.ServeHTTP(w, r)
					})
				}))
			mux := chi.NewRouter()
			if err := b.Mount(mux); err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, test.target, nil))
			if w.Code != http.StatusOK || w.Body.String() != test.want {
				t.Fatalf("%d %q; want %q", w.Code, w.Body.String(), test.want)
			}
		})
	}
}
