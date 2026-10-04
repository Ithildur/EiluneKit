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

func TestFromRequestHeaderPriority(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com", nil)
	req.RemoteAddr = "192.0.2.10:1234"
	req.Header.Set("X-Forwarded-For", "198.51.100.1, 192.0.2.20")
	req.Header.Set("X-Real-IP", "198.51.100.2")
	req.Header.Set("Forwarded", "for=198.51.100.3;proto=https, for=192.0.2.20")
	req.Header.Set("True-Client-IP", "198.51.100.4")
	req.Header.Set("CF-Connecting-IP", "198.51.100.5")
	opts := clientip.Options{TrustedProxies: []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")}}
	for _, tc := range []struct{ header, want string }{
		{"X-Forwarded-For", "198.51.100.1"},
		{"X-Real-IP", "198.51.100.2"},
		{"Forwarded", "198.51.100.3"},
		{"True-Client-IP", "198.51.100.4"},
		{"CF-Connecting-IP", "198.51.100.5"},
	} {
		ip, ok := clientip.FromRequest(req, opts)
		if !ok || ip.String() != tc.want {
			t.Fatalf("with %s: got %v, want %s", tc.header, ip, tc.want)
		}
		req.Header.Del(tc.header)
	}
}

func TestFromRequestHeaderOverrides(t *testing.T) {
	for _, tc := range []struct {
		name    string
		headers []string
		remote  string
		want    string
	}{
		{"reordered", []string{"Forwarded", "X-Forwarded-For"}, "192.0.2.10:1234", "198.51.100.3"},
		{"case insensitive", []string{"x-forwarded-for"}, "192.0.2.10:1234", "198.51.100.1"},
		{"invalid falls through", []string{"X-Bad-IP", "X-Real-IP"}, "192.0.2.10:1234", "198.51.100.2"},
		{"custom single IP", []string{"X-Client-IP"}, "192.0.2.10:1234", "198.51.100.4"},
		{"no implicit fallback", []string{"X-Missing-IP"}, "192.0.2.10:1234", "192.0.2.10"},
		{"empty disables headers", []string{}, "192.0.2.10:1234", "192.0.2.10"},
		{"untrusted peer", []string{"Forwarded"}, "203.0.113.10:1234", "203.0.113.10"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://example.com", nil)
			req.RemoteAddr = tc.remote
			req.Header.Set("X-Forwarded-For", "198.51.100.1, 192.0.2.20")
			req.Header.Set("X-Real-IP", "198.51.100.2")
			req.Header.Set("Forwarded", "for=198.51.100.3")
			req.Header.Set("X-Client-IP", "198.51.100.4")
			req.Header.Set("X-Bad-IP", "invalid")
			ip, ok := clientip.FromRequest(req, clientip.Options{
				TrustedProxies: []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")},
				Headers:        tc.headers,
			})
			if !ok || ip.String() != tc.want {
				t.Fatalf("got %v, want %s", ip, tc.want)
			}
		})
	}
}
