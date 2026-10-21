package captchahttp_test

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ithildur/EiluneKit/captcha"
	captchahttp "github.com/Ithildur/EiluneKit/captcha/http"
	"github.com/Ithildur/EiluneKit/http/response"
)

func TestMiddleware(t *testing.T) {
	for _, tt := range []struct {
		name, token string
		err         error
		status      int
		code        string
	}{
		{"success", "token", nil, 204, ""},
		{"missing", "", nil, 400, "captcha_required"},
		{"blank", " ", nil, 400, "captcha_required"},
		{"rejected", "token", fmt.Errorf("wrapped: %w", captcha.ErrRejected), 403, "captcha_rejected"},
		{"misconfigured", "token", captcha.ErrMisconfigured, 500, "captcha_misconfigured"},
		{"unavailable", "token", captcha.ErrUnavailable, 503, "captcha_unavailable"},
		{"unknown", "token", errors.New("secret-token"), 503, "captcha_unavailable"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls, nextCalls := 0, 0
			mw, err := captchahttp.New(captchahttp.Options{Enabled: true, Verify: func(ctx context.Context, token string) error {
				calls++
				if token != tt.token {
					t.Fatalf("unexpected token %q", token)
				}
				return tt.err
			}})
			if err != nil {
				t.Fatal(err)
			}
			h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextCalls++
				body, err := io.ReadAll(r.Body)
				if err != nil || string(body) != "original body" {
					t.Fatalf("request body changed: %q, %v", body, err)
				}
				w.WriteHeader(204)
			}))
			r := httptest.NewRequest(http.MethodPost, "/submit", strings.NewReader("original body"))
			r.Header.Set(captchahttp.TokenHeader, tt.token)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tt.status {
				t.Fatalf("status=%d, want %d", w.Code, tt.status)
			}
			wantCalls := 1
			if strings.TrimSpace(tt.token) == "" {
				wantCalls = 0
			}
			if calls != wantCalls {
				t.Fatalf("verification calls=%d, want %d", calls, wantCalls)
			}
			if tt.status == 204 {
				if nextCalls != 1 {
					t.Fatal("handler did not run")
				}
			} else {
				if nextCalls != 0 {
					t.Fatal("failure reached handler")
				}
				var payload response.ErrorResponse
				if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
					t.Fatal(err)
				}
				if payload.Code != tt.code || strings.Contains(w.Body.String(), "secret-token") {
					t.Fatalf("unexpected error: %s", w.Body.String())
				}
			}
		})
	}
}

func TestEnablement(t *testing.T) {
	for _, verify := range []func(context.Context, string) error{nil, func(context.Context, string) error { t.Fatal("disabled verifier called"); return nil }} {
		mw, err := captchahttp.New(captchahttp.Options{Verify: verify})
		if err != nil || mw != nil {
			t.Fatalf("disabled middleware: %v", err)
		}
	}
	if _, err := captchahttp.New(captchahttp.Options{Enabled: true}); !errors.Is(err, captcha.ErrMisconfigured) {
		t.Fatalf("missing verifier: %v", err)
	}
}
