package session_test

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

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
				TrustedProxies: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")},
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
