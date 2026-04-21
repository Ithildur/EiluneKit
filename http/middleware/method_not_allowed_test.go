package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
