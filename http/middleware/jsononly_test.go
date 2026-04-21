// Contract tests for JSONOnly middleware.
// JSONOnly 的契约测试：只允许 JSON body，空 body 放行。
package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ithildur/EiluneKit/http/response"
)

func TestJSONOnly_AllowsEmptyBody(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	handler := JSONOnly(next)
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); body != "ok" {
		t.Fatalf("expected body ok, got %q", body)
	}
}

func TestJSONOnly_RejectsNonJSON(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := JSONOnly(next)
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("nope"))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected status 415, got %d", rec.Code)
	}

	var payload response.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if payload.Code != "unsupported_media_type" {
		t.Fatalf("expected code unsupported_media_type, got %q", payload.Code)
	}
	if payload.Message != "content-type must be application/json" {
		t.Fatalf("expected message content-type must be application/json, got %q", payload.Message)
	}
}
