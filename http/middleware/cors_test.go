package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCORSPermissions(t *testing.T) {
	methods := []string{"GET", "POST"}
	called := false
	h := CORS(CORSOptions{
		AllowedOrigins: []string{"https://app.example"}, AllowedMethods: methods,
		AllowedHeaders: []string{"Authorization"}, ExposedHeaders: []string{"X-Request-Id"},
		AllowCredentials: true, MaxAge: 5 * time.Minute,
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(404)
	}))
	methods[1] = "DELETE"
	for _, test := range []struct {
		origin, method, header string
		allowed                bool
	}{
		{"https://app.example", "POST", "authorization", true},
		{"https://other.example", "POST", "authorization", false},
		{"https://app.example", "DELETE", "authorization", false},
		{"https://app.example", "POST", "x-private", false},
	} {
		called = false
		r := httptest.NewRequest("OPTIONS", "/missing", nil)
		r.Header.Set("Origin", test.origin)
		r.Header.Set("Access-Control-Request-Method", test.method)
		r.Header.Set("Access-Control-Request-Headers", test.header)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if called || w.Code != http.StatusNoContent {
			t.Fatalf("preflight reached handler: %t, status %d", called, w.Code)
		}
		if got := w.Header().Get("Access-Control-Allow-Origin"); (got != "") != test.allowed {
			t.Errorf("%+v: origin header %q", test, got)
		}
		if test.allowed && (w.Header().Get("Access-Control-Allow-Credentials") != "true" || w.Header().Get("Access-Control-Max-Age") != "300") {
			t.Errorf("preflight headers: %v", w.Header())
		}
	}
	r := httptest.NewRequest("GET", "/missing", nil)
	r.Header.Set("Origin", "https://app.example")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 404 || w.Header().Get("Access-Control-Allow-Origin") != "https://app.example" || !strings.Contains(w.Header().Get("Access-Control-Expose-Headers"), "X-Request-Id") {
		t.Fatalf("error response lost CORS headers: %d %v", w.Code, w.Header())
	}
}

func TestCORSRequiresExplicitOrigins(t *testing.T) {
	for _, origins := range [][]string{nil, {"*"}} {
		h := CORS(CORSOptions{AllowedOrigins: origins})(http.NotFoundHandler())
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Origin", "https://other.example")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if got := w.Header().Get("Access-Control-Allow-Origin"); (got == "*") != (len(origins) != 0) {
			t.Errorf("origins %v: headers %v", origins, w.Header())
		}
	}
	defer func() {
		if recover() == nil {
			t.Fatal("wildcard credentials accepted")
		}
	}()
	CORS(CORSOptions{AllowedOrigins: []string{"*"}, AllowCredentials: true})
}
