package routes_test

import (
	"net/http"
	"testing"

	"github.com/Ithildur/EiluneKit/http/routes"
	"github.com/go-chi/chi/v5"
)

func TestRejectsNoncanonicalRoutePaths(t *testing.T) {
	for _, path := range []string{"users", " /users", "/users ", "//users"} {
		t.Run(path, func(t *testing.T) {
			route := routes.Route{Method: http.MethodGet, Path: path, Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})}
			mustPanic(t, func() { routes.NewBlueprint().Add(route) })
			mustPanic(t, func() { routes.WithPrefix("/api", []routes.Route{route}) })
			if err := routes.Mount(chi.NewRouter(), "/api", []routes.Route{route}); err == nil {
				t.Fatal("mount accepted invalid path")
			}
			if _, err := routes.ExportJSON([]routes.Route{route}); err == nil {
				t.Fatal("JSON export accepted invalid path")
			}
			if _, err := routes.ExportMarkdown([]routes.Route{route}); err == nil {
				t.Fatal("Markdown export accepted invalid path")
			}
		})
	}
}

func TestRejectsNoncanonicalPrefixes(t *testing.T) {
	for _, prefix := range []string{"api", "/api/", "/", "//api", " /api", "/api "} {
		t.Run(prefix, func(t *testing.T) {
			mustPanic(t, func() { routes.NewBlueprint().Include(prefix, routes.NewBlueprint()) })
			mustPanic(t, func() { routes.NewBlueprint().RoutesAt(prefix) })
			mustPanic(t, func() { routes.NewRouter().Include(prefix, nil) })
			if err := routes.NewBlueprint().MountAt(chi.NewRouter(), prefix); err == nil {
				t.Fatal("mount accepted invalid prefix")
			}
		})
	}
}
