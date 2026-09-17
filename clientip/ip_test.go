package clientip_test

import (
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/Ithildur/EiluneKit/clientip"
)

func TestFromRequest(t *testing.T) {
	tests := []struct {
		name      string
		xff       string
		forwarded string
		trusted   bool
		wantIP    string
	}{
		{
			name:   "uses_remote_addr_by_default",
			xff:    "198.51.100.7",
			wantIP: "192.0.2.10",
		},
		{
			name:    "trusted_proxy_uses_forwarded_headers",
			xff:     "198.51.100.7, 192.0.2.10",
			trusted: true,
			wantIP:  "198.51.100.7",
		},
		{name: "forwarded_ipv6_port", forwarded: `for="[2001:db8::1]:4711"`, trusted: true, wantIP: "2001:db8::1"},
		{name: "forwarded_ipv6", forwarded: `for="[2001:db8::1]"`, trusted: true, wantIP: "2001:db8::1"},
		{name: "forwarded_ipv4_port", forwarded: `for="198.51.100.7:4711"`, trusted: true, wantIP: "198.51.100.7"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://example.com", nil)
			req.RemoteAddr = "192.0.2.10:1234"
			req.Header.Set("X-Forwarded-For", tc.xff)
			req.Header.Set("Forwarded", tc.forwarded)
			var options clientip.Options
			if tc.trusted {
				options.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")}
			}

			ip, ok := clientip.FromRequest(req, options)
			if !ok {
				t.Fatalf("expected ip")
			}
			if got := ip.String(); got != tc.wantIP {
				t.Fatalf("expected %s, got %s", tc.wantIP, got)
			}
		})
	}
}
