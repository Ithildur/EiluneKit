package routes_test

import (
	"bytes"
	"compress/gzip"
	"encoding/json/v2"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/Ithildur/EiluneKit/http/middleware"
	"github.com/Ithildur/EiluneKit/http/response"
	"github.com/Ithildur/EiluneKit/http/routes"
	"github.com/go-chi/chi/v5"
)

func TestNewHandlerRequestBoundary(t *testing.T) {
	var calls []string
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	trace := func(name string) routes.Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls = append(calls, name+" in")
				defer func() { calls = append(calls, name+" out") }()
				next.ServeHTTP(w, r)
			})
		}
	}
	api := routes.NewBlueprint(routes.DefaultMiddleware(trace("endpoint")))
	api.Get("/items/{id}", "", func(w http.ResponseWriter, r *http.Request, id string) {
		_, _ = io.WriteString(w, id)
	})
	api.Delete("/items/{id}", "", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	api.Post("/items", "", func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	api.Get("/private", "", func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("unauthenticated request reached endpoint")
	}, routes.Auth(routes.AuthRequired))
	api.Get("/panic", "", func(http.ResponseWriter, *http.Request) { panic("endpoint failure") })
	boundary := func(next http.Handler) http.Handler {
		limited := middleware.LimitBody(4)(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
				w.Header().Set("Cache-Control", "no-store")
				limited.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
	page := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "page") })
	h, err := routes.NewHandler(api.RoutesAt("/api"), routes.HandlerOptions{
		Middleware: []routes.Middleware{
			middleware.RequestID,
			middleware.AccessLog(middleware.AccessLogOptions{Logger: logger}),
			trace("outer"), nil, trace("inner"), boundary,
			middleware.CORS(middleware.CORSOptions{
				AllowedOrigins: []string{"https://app.example"},
				AllowedMethods: []string{"GET", "POST", "DELETE"},
			}),
			middleware.Compress(middleware.CompressOptions{MinSize: 1}),
			middleware.Recover(middleware.RecoverOptions{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}),
		},
		NotFound: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
				response.WriteJSONError(w, 404, "not_found", "resource not found")
				return
			}
			page.ServeHTTP(w, r)
		}),
		MethodNotAllowed: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !slices.Equal(w.Header().Values("Allow"), []string{"DELETE", "GET"}) {
				t.Errorf("Allow not ready in callback: %v", w.Header().Values("Allow"))
			}
			response.WriteJSONError(w, 405, "method_not_allowed", "method not allowed")
		}),
		Unauthorized: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response.WriteJSONError(w, 401, "sign_in", "sign in required")
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		method, path, body, code string
		status                   int
		endpoint                 bool
	}{
		{"GET", "/api/items/42", "", "", 200, true},
		{"POST", "/api/items", "1234", "", 204, true},
		{"POST", "/api/items", "12345", "", 413, true},
		{"GET", "/api", "", "not_found", 404, false},
		{"GET", "/api/", "", "not_found", 404, false},
		{"POST", "/api/missing", "12345", "not_found", 404, false},
		{"POST", "/api/items/42", "12345", "method_not_allowed", 405, false},
		{"GET", "/api/private", "", "sign_in", 401, true},
		{"GET", "/api/panic", "", "", 500, true},
		{"OPTIONS", "/api/unregistered", "", "", 204, false},
		{"GET", "/apix", "", "", 200, false},
		{"GET", "/", "", "", 200, false},
		{"GET", "/page", "", "", 200, false},
	} {
		t.Run(test.method+test.path+test.body, func(t *testing.T) {
			calls = nil
			logs.Reset()
			w := httptest.NewRecorder()
			r := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			r.ContentLength = -1
			r.Header.Set("Origin", "https://app.example")
			r.Header.Set("Accept-Encoding", "gzip")
			r.Header.Set("X-Request-Id", "boundary-request")
			if test.method == "OPTIONS" {
				r.Header.Set("Access-Control-Request-Method", "POST")
			}
			h.ServeHTTP(w, r)
			if w.Code != test.status {
				t.Fatalf("status %d, want %d: %s", w.Code, test.status, w.Body.String())
			}
			want := []string{"outer in", "inner in"}
			if test.endpoint {
				want = append(want, "endpoint in", "endpoint out")
			}
			want = append(want, "inner out", "outer out")
			if !slices.Equal(calls, want) {
				t.Errorf("calls %v, want %v", calls, want)
			}
			if w.Header().Get("Access-Control-Allow-Origin") != "https://app.example" {
				t.Errorf("missing CORS response headers: %v", w.Header())
			}
			body := w.Body.Bytes()
			if len(body) > 0 {
				if w.Header().Get("Content-Encoding") != "gzip" {
					t.Fatalf("response was not compressed: %v", w.Header())
				}
				reader, err := gzip.NewReader(bytes.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				defer reader.Close()
				body, err = io.ReadAll(reader)
				if err != nil {
					t.Fatal(err)
				}
			}
			if !strings.HasPrefix(test.path, "/api/") && test.path != "/api" {
				if w.Header().Get("Cache-Control") != "" || string(body) != "page" {
					t.Fatalf("API boundary affected page: %v %s", w.Header(), body)
				}
			} else if w.Header().Get("Cache-Control") != "no-store" {
				t.Error("missing no-store")
			}
			if test.code != "" {
				var payload response.ErrorResponse
				if err := json.Unmarshal(body, &payload); err != nil || payload.Code != test.code {
					t.Errorf("response %s, error %v", body, err)
				}
			}
			var record struct {
				Status    int    `json:"status"`
				RequestID string `json:"request_id"`
				Aborted   bool   `json:"aborted"`
			}
			if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
				t.Fatal(err)
			}
			if record.Status != test.status || record.RequestID != "boundary-request" || record.Aborted {
				t.Fatalf("unexpected access log: %s", logs.String())
			}
		})
	}
}

func TestNewHandlerDefaultsAndSnapshot(t *testing.T) {
	b := routes.NewBlueprint()
	b.Get("/item", "", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	b.Get("/private", "", func(w http.ResponseWriter, r *http.Request) { t.Error("auth bypass") }, routes.Auth(routes.AuthRequired))
	list := b.Routes()
	mws := []routes.Middleware{func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Boundary", "yes")
			next.ServeHTTP(w, r)
		})
	}}
	h, err := routes.NewHandler(list, routes.HandlerOptions{Middleware: mws})
	if err != nil {
		t.Fatal(err)
	}
	list[0].Method, list[0].Path, list[0].Handler = "POST", "/changed", http.NotFoundHandler()
	mws[0] = nil
	for _, test := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/item", 204}, {"HEAD", "/item", 405},
		{"GET", "/missing", 404}, {"GET", "/private", 401},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(test.method, test.path, nil))
		if w.Code != test.status || w.Header().Get("X-Boundary") != "yes" {
			t.Errorf("%s %s: %d %v", test.method, test.path, w.Code, w.Header())
		}
		if test.status == 405 && (w.Header().Get("Allow") != "GET" || w.Body.Len() != 0) {
			t.Errorf("default 405: %v %s", w.Header(), w.Body.String())
		}
	}
}

func TestNewHandlerEmptyRoutes(t *testing.T) {
	h, err := routes.NewHandler(nil, routes.HandlerOptions{
		Middleware: []routes.Middleware{func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Boundary", "yes")
				next.ServeHTTP(w, r)
			})
		}},
		NotFound: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(410) }),
	})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/missing", nil))
	if w.Code != 410 || w.Header().Get("X-Boundary") != "yes" {
		t.Fatalf("empty router bypassed boundary: %d %v", w.Code, w.Header())
	}
}

func TestNewHandlerAllowUsesMatchingRoutes(t *testing.T) {
	chi.RegisterMethod("PROPFIND")
	b := routes.NewBlueprint()
	endpoint := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }
	b.Get("/items/{id}", "", endpoint)
	b.Handle("PROPFIND", "/items/{id:[0-9]+}", "", endpoint)
	b.Post("/other", "", endpoint)
	h, err := routes.NewHandler(b.Routes(), routes.HandlerOptions{
		MethodNotAllowed: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if chi.URLParam(r, "id") != "" || chi.RouteContext(r.Context()).RoutePattern() != "" {
				t.Error("Allow probing changed the request route")
			}
			w.WriteHeader(http.StatusMethodNotAllowed)
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{"/items/42": "GET,PROPFIND", "/items/name": "GET"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("PATCH", path, nil))
		if w.Code != 405 || strings.Join(w.Header().Values("Allow"), ",") != want {
			t.Errorf("%s: %d %v", path, w.Code, w.Header())
		}
	}
}

func TestNewHandlerRejectsInvalidRoutesBeforeWrapping(t *testing.T) {
	wrapped := false
	opts := routes.HandlerOptions{Middleware: []routes.Middleware{func(next http.Handler) http.Handler {
		wrapped = true
		return next
	}}}
	h, err := routes.NewHandler([]routes.Route{{Method: "GET", Path: "invalid", Handler: http.NotFoundHandler()}}, opts)
	if err == nil || h != nil || wrapped {
		t.Fatalf("invalid routes: handler %v, error %v, wrapped %v", h, err, wrapped)
	}
}
