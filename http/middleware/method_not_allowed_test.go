package middleware

import (
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/Ithildur/EiluneKit/http/response"

	"github.com/go-chi/chi/v5"
)

func TestMethodNotAllowedResponder_EmitsAllowAndJSON(t *testing.T) {
	r := chi.NewRouter()
	r.MethodNotAllowed(MethodNotAllowedResponder(r))
	r.Get("/x", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
	if allow := rec.Header().Get("Allow"); allow == "" {
		t.Fatal("expected Allow header to be set")
	}

	var payload response.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if payload.Code != "method_not_allowed" {
		t.Fatalf("expected code method_not_allowed, got %q", payload.Code)
	}
	if payload.Message != "method not allowed" {
		t.Fatalf("expected message method not allowed, got %q", payload.Message)
	}
}

func TestAllowedMethodsPreservesRequestRoute(t *testing.T) {
	mux := chi.NewRouter()
	mux.Get("/files/{id}", func(w http.ResponseWriter, r *http.Request) {
		before := chi.RouteContext(r.Context()).RoutePattern()
		allowed := AllowedMethodsForRoute(mux, r)
		if !slices.Equal(allowed, []string{http.MethodGet, http.MethodPut, http.MethodDelete}) {
			t.Errorf("unexpected allowed methods: %v", allowed)
		}
		if chi.URLParam(r, "id") != "42" || chi.RouteContext(r.Context()).RoutePattern() != before {
			t.Error("method probing changed the request route")
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.Put("/files/{id:[0-9]+}", func(http.ResponseWriter, *http.Request) {})
	mux.Delete("/files/*", func(http.ResponseWriter, *http.Request) {})
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/files/42", nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("unexpected status: %d", w.Code)
	}
}
