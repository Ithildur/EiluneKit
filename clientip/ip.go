// Package clientip provides client IP helpers.
// Package clientip 提供客户端 IP 辅助函数。
package clientip

import (
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"
)

// Options configures forwarded-header trust.
// Options 配置转发头信任边界。
type Options struct {
	TrustedProxies []netip.Prefix
	// Headers lists accepted client IP headers in priority order.
	// Nil uses X-Forwarded-For, X-Real-IP, Forwarded, True-Client-IP, then CF-Connecting-IP.
	// An empty non-nil slice disables forwarded headers. Other names accept a single IP.
	// Headers 按优先级列出接受的客户端 IP 头。
	// Nil 依次使用 X-Forwarded-For、X-Real-IP、Forwarded、True-Client-IP、CF-Connecting-IP。
	// 非 nil 空切片禁用转发头。其他头名称按单个 IP 解析。
	Headers []string
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
	remote, ok := FromRemote(r.RemoteAddr)
	if !ok {
		return netip.Addr{}, false
	}
	if !isTrustedIP(remote, opts.TrustedProxies) {
		return remote, true
	}

	for _, ip := range slices.Backward(forwardedCandidates(r.Header, opts.Headers)) {
		if !isTrustedIP(ip, opts.TrustedProxies) {
			return ip, true
		}
	}

	return remote, true
}

func forwardedCandidates(header http.Header, headers []string) []netip.Addr {
	if headers == nil {
		headers = []string{"X-Forwarded-For", "X-Real-IP", "Forwarded", "True-Client-IP", "CF-Connecting-IP"}
	}
	for _, name := range headers {
		var ips []netip.Addr
		switch http.CanonicalHeaderKey(name) {
		case "Forwarded":
			ips = parseForwardedForList(header.Get(name))
		case "X-Forwarded-For":
			ips = parseXForwardedForList(header.Get(name))
		default:
			if ip, ok := parseSingleIPHeader(header.Get(name)); ok {
				ips = []netip.Addr{ip}
			}
		}
		if len(ips) > 0 {
			return ips
		}
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
		for param := range strings.SplitSeq(part, ";") {
			param = strings.TrimSpace(param)
			if len(param) < 4 {
				continue
			}
			if !strings.HasPrefix(strings.ToLower(param), "for=") {
				continue
			}
			val := strings.TrimSpace(param[4:])
			val = strings.Trim(val, "\"")
			val = stripPort(val)
			val = strings.TrimPrefix(val, "[")
			val = strings.TrimSuffix(val, "]")
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
