package clientip_test

import (
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/Ithildur/EiluneKit/clientip"
)

func TestFromRequest(t *testing.T) {
	tests := []struct {
		name    string
		xff     string
		options clientip.Options
		wantIP  string
	}{
		{
			name:    "uses_remote_addr_by_default",
			xff:     "198.51.100.7",
			options: clientip.Options{},
			wantIP:  "192.0.2.10",
		},
		{
			name: "trusted_proxy_uses_forwarded_headers",
			xff:  "198.51.100.7, 192.0.2.10",
			options: clientip.Options{
				TrustedProxies: []netip.Prefix{mustPrefix(t, "192.0.2.0/24")},
			},
			wantIP: "198.51.100.7",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://example.com", nil)
			req.RemoteAddr = "192.0.2.10:1234"
			req.Header.Set("X-Forwarded-For", tc.xff)

			ip, ok := clientip.FromRequest(req, tc.options)
			if !ok {
				t.Fatalf("expected ip")
			}
			if got := ip.String(); got != tc.wantIP {
				t.Fatalf("expected %s, got %s", tc.wantIP, got)
			}
		})
	}
}

func mustPrefix(t *testing.T, raw string) netip.Prefix {
	t.Helper()
	prefix, err := netip.ParsePrefix(raw)
	if err != nil {
		t.Fatalf("parse prefix %q: %v", raw, err)
	}
	return prefix
}
