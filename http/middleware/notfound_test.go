// Contract tests for NotFoundHandler middleware.
// NotFoundHandler 的契约测试：/api 前缀返回 JSON 404。
package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Ithildur/EiluneKit/http/response"
)

func TestNotFoundHandler_APIPathsReturnJSON(t *testing.T) {
	static := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("static"))
	})
	handler := NotFoundHandler(static)

	for _, path := range []string{"/api", "/api/x"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("path %s: expected 404, got %d", path, rec.Code)
		}

		var payload response.ErrorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("path %s: unmarshal error: %v", path, err)
		}
		if payload.Code != "not_found" {
			t.Fatalf("path %s: expected code not_found, got %q", path, payload.Code)
		}
		if payload.Message != "resource not found" {
			t.Fatalf("path %s: expected message resource not found, got %q", path, payload.Message)
		}
	}
}

func TestNotFoundHandler_NonAPIPathsUseStatic(t *testing.T) {
	static := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("static"))
	})
	handler := NotFoundHandler(static)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); body != "static" {
		t.Fatalf("expected body static, got %q", body)
	}
}
