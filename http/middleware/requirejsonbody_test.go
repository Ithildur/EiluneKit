package middleware

import (
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ithildur/EiluneKit/http/response"
)

func TestRequireJSONBody(t *testing.T) {
	for _, test := range []struct {
		name, body, contentType string
		status                  int
	}{
		{"empty", "", "", 204},
		{"JSON", "{}", "application/json", 204},
		{"JSON with charset", "{}", "application/json; charset=utf-8", 204},
		{"JSON with whitespace", "{}", "application/json ; charset=utf-8", 204},
		{"JSON with mixed case", "{}", "Application/JSON", 204},
		{"invalid charset", "{}", "application/json; charset", 415},
		{"conflicting charset", "{}", "application/json; charset=utf-8; charset=ascii", 415},
		{"non-JSON", "nope", "text/plain", 415},
		{"missing content type", "{}", "", 415},
	} {
		t.Run(test.name, func(t *testing.T) {
			called := false
			h := RequireJSONBody(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(204)
			}))
			r := httptest.NewRequest("POST", "/x", strings.NewReader(test.body))
			r.Header.Set("Content-Type", test.contentType)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != test.status || called != (test.status == 204) {
				t.Fatalf("status %d, handler called %t", w.Code, called)
			}
			if test.status == 415 {
				var payload response.ErrorResponse
				if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
					t.Fatal(err)
				}
				if payload.Code != "unsupported_media_type" || payload.Message != "content-type must be application/json" {
					t.Fatalf("unexpected response: %s", w.Body.String())
				}
			}
		})
	}
}

func BenchmarkRequireJSONBody(b *testing.B) {
	h := RequireJSONBody(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	for _, contentType := range []string{"application/json", "application/json ; charset=utf-8"} {
		b.Run(contentType, func(b *testing.B) {
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}"))
			r.Header.Set("Content-Type", contentType)
			w := httptest.NewRecorder()
			b.ReportAllocs()
			for b.Loop() {
				h.ServeHTTP(w, r)
			}
		})
	}
}
