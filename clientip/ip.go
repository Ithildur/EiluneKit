// Package clientip provides client IP helpers.
// Package clientip 提供客户端 IP 辅助函数。
package clientip

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// Options configures forwarded-header trust.
// Options 配置转发头信任边界。
type Options struct {
	TrustedProxies []netip.Prefix
}

// FromRemote parses an IP from remoteAddr.
// FromRemote 从 remoteAddr 解析 IP。
func FromRemote(remoteAddr string) (netip.Addr, bool) {
	remote := strings.TrimSpace(remoteAddr)
	if remote == "" {
		return netip.Addr{}, false
	}
	if ip, err := netip.ParseAddr(remote); err == nil {
		return ip, true
	}
	host, _, err := net.SplitHostPort(remote)
	if err == nil {
		host = strings.TrimSpace(host)
		if ip, err := netip.ParseAddr(host); err == nil {
			return ip, true
		}
	}
	return netip.Addr{}, false
}

// FromRequest returns the client IP for r.
// Call FromRequest(r, Options{TrustedProxies: ...}) when forwarded headers are trusted.
// FromRequest 返回 r 的客户端 IP。
// 需要信任转发头时，调用 FromRequest(r, Options{TrustedProxies: ...})。
//
// Example / 示例:
//
//	ip, ok := clientip.FromRequest(r, clientip.Options{})
func FromRequest(r *http.Request, opts Options) (netip.Addr, bool) {
	if r == nil {
		return netip.Addr{}, false
	}
	if len(opts.TrustedProxies) > 0 {
		return fromTrustedRequest(r, opts.TrustedProxies)
	}
	return FromRemote(r.RemoteAddr)
}

func fromTrustedRequest(r *http.Request, trusted []netip.Prefix) (netip.Addr, bool) {
	if r == nil {
		return netip.Addr{}, false
	}
	remote, ok := FromRemote(r.RemoteAddr)
	if !ok {
		return netip.Addr{}, false
	}
	if len(trusted) == 0 || !isTrustedIP(remote, trusted) {
		return remote, true
	}

	candidates := forwardedCandidates(r)
	if len(candidates) == 0 {
		return remote, true
	}

	for i := len(candidates) - 1; i >= 0; i-- {
		ip := candidates[i]
		if !isTrustedIP(ip, trusted) {
			return ip, true
		}
	}

	return remote, true
}

func forwardedCandidates(r *http.Request) []netip.Addr {
	if r == nil {
		return nil
	}
	if ips := parseForwardedForList(r.Header.Get("Forwarded")); len(ips) > 0 {
		return ips
	}
	if ips := parseXForwardedForList(r.Header.Get("X-Forwarded-For")); len(ips) > 0 {
		return ips
	}
	if ip, ok := parseSingleIPHeader(r.Header.Get("True-Client-IP")); ok {
		return []netip.Addr{ip}
	}
	if ip, ok := parseSingleIPHeader(r.Header.Get("CF-Connecting-IP")); ok {
		return []netip.Addr{ip}
	}
	if ip, ok := parseSingleIPHeader(r.Header.Get("X-Real-IP")); ok {
		return []netip.Addr{ip}
	}
	return nil
}

func parseForwardedForList(raw string) []netip.Addr {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]netip.Addr, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		params := strings.Split(part, ";")
		for _, param := range params {
			param = strings.TrimSpace(param)
			if len(param) < 4 {
				continue
			}
			if !strings.HasPrefix(strings.ToLower(param), "for=") {
				continue
			}
			val := strings.TrimSpace(param[4:])
			val = strings.Trim(val, "\"")
			val = strings.TrimPrefix(val, "[")
			val = strings.TrimSuffix(val, "]")
			val = stripPort(val)
			if ip, err := netip.ParseAddr(val); err == nil {
				out = append(out, ip)
				break
			}
		}
	}
	return out
}

func parseXForwardedForList(raw string) []netip.Addr {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]netip.Addr, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if ip, err := netip.ParseAddr(p); err == nil {
			out = append(out, ip)
		}
	}
	return out
}

func parseSingleIPHeader(raw string) (netip.Addr, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return netip.Addr{}, false
	}
	ip, err := netip.ParseAddr(raw)
	if err != nil {
		return netip.Addr{}, false
	}
	return ip, true
}

func stripPort(host string) string {
	if host == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

func isTrustedIP(ip netip.Addr, trusted []netip.Prefix) bool {
	if !ip.IsValid() {
		return false
	}
	for _, p := range trusted {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}
