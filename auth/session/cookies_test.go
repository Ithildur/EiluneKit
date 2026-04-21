package session_test

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/Ithildur/EiluneKit/auth/session"
)

func TestDefaultCookieConfig(t *testing.T) {
	tests := []struct {
		name     string
		remote   string
		trust    session.CookieTrustOptions
		secure   bool
		sameSite http.SameSite
	}{
		{
			name:     "does_not_trust_forwarded_proto_by_default",
			remote:   "198.51.100.10:1234",
			trust:    session.CookieTrustOptions{},
			secure:   false,
			sameSite: http.SameSiteLaxMode,
		},
		{
			name:   "trusts_forwarded_proto_from_trusted_proxy",
			remote: "127.0.0.1:1234",
			trust: session.CookieTrustOptions{
				TrustedProxies: []netip.Prefix{mustPrefix(t, "127.0.0.1/32")},
			},
			secure:   true,
			sameSite: http.SameSiteNoneMode,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
			req.RemoteAddr = tc.remote
			req.Header.Set("X-Forwarded-Proto", "https")

			cfg := session.DefaultCookieConfig(req, tc.trust)
			if got := cfg.Secure; got != tc.secure {
				t.Fatalf("expected Secure=%v, got %v", tc.secure, got)
			}
			if got := cfg.SameSite; got != tc.sameSite {
				t.Fatalf("expected SameSite=%v, got %v", tc.sameSite, got)
			}
		})
	}
}

func TestSetRefreshCookieLifetime(t *testing.T) {
	exp := time.Now().UTC().Add(time.Hour)

	t.Run("persistent", func(t *testing.T) {
		rec := httptest.NewRecorder()
		session.SetRefreshCookie(rec, "refresh-token", exp, session.CookieConfig{
			Name: session.DefaultRefreshCookieName,
			Path: "/",
		})

		cookie := rec.Result().Cookies()[0]
		if cookie.MaxAge <= 0 {
			t.Fatalf("expected persistent cookie MaxAge > 0, got %d", cookie.MaxAge)
		}
		if cookie.Expires.IsZero() {
			t.Fatal("expected persistent cookie expiration")
		}
	})

	t.Run("session_only", func(t *testing.T) {
		rec := httptest.NewRecorder()
		session.SetRefreshCookie(rec, "refresh-token", exp, session.CookieConfig{
			Name:        session.DefaultRefreshCookieName,
			Path:        "/",
			SessionOnly: true,
		})

		cookie := rec.Result().Cookies()[0]
		if cookie.MaxAge != 0 {
			t.Fatalf("expected session cookie MaxAge=0, got %d", cookie.MaxAge)
		}
		if !cookie.Expires.IsZero() {
			t.Fatalf("expected session cookie without expiration, got %s", cookie.Expires.UTC().Format(time.RFC3339))
		}
	})
}

func mustPrefix(t *testing.T, raw string) netip.Prefix {
	t.Helper()
	prefix, err := netip.ParsePrefix(raw)
	if err != nil {
		t.Fatalf("parse prefix %q: %v", raw, err)
	}
	return prefix
}
