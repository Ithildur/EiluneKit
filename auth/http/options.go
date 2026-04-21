package authhttp

import (
	"net/netip"
	"strings"

	authsession "github.com/Ithildur/EiluneKit/auth/session"
)

const defaultMaxBodyBytes int64 = 1 << 20
const defaultAuthBasePath = "/auth"

// Options configures NewHandler.
// Options 配置 NewHandler。
type Options struct {
	// LoginAuthenticator validates login credentials.
	// LoginAuthenticator 校验登录凭据。
	LoginAuthenticator LoginAuthenticator
	// BasePath is the auth route prefix.
	// BasePath 是认证路由前缀。
	BasePath string
	// RefreshCookiePath defaults to BasePath when empty.
	// RefreshCookiePath 为空时默认等于 BasePath。
	RefreshCookiePath string
	// CSRFCookiePath defaults to "/".
	// CSRFCookiePath 默认为 "/"。
	CSRFCookiePath string

	// RefreshCookieName is the refresh cookie name.
	// RefreshCookieName 是 refresh cookie 名。
	RefreshCookieName string
	// CSRFCookieName is the CSRF cookie name.
	// CSRFCookieName 是 CSRF cookie 名。
	CSRFCookieName string
	// CSRFHeaderName is the CSRF request header name.
	// CSRFHeaderName 是 CSRF 请求头名。
	CSRFHeaderName string
	// TrustedProxies enables forwarded-header trust.
	// TrustedProxies 启用转发头信任。
	TrustedProxies []netip.Prefix

	// MaxBodyBytes limits the login body size.
	// MaxBodyBytes 限制登录请求体大小。
	MaxBodyBytes int64
	// RateLimit configures login rate limiting.
	// RateLimit 配置登录限流。
	RateLimit *RateLimitOptions
}

// DefaultOptions returns the default handler options.
// DefaultOptions 返回默认 handler 选项。
func DefaultOptions() Options {
	rate := DefaultRateLimitOptions()
	return Options{
		BasePath:          defaultAuthBasePath,
		RefreshCookiePath: "",
		CSRFCookiePath:    "/",
		RefreshCookieName: authsession.DefaultRefreshCookieName,
		CSRFCookieName:    authsession.DefaultCSRFCookieName,
		CSRFHeaderName:    authsession.DefaultCSRFHeaderName,
		MaxBodyBytes:      defaultMaxBodyBytes,
		RateLimit:         &rate,
	}
}

func applyOptions(base, override Options) Options {
	if override.LoginAuthenticator != nil {
		base.LoginAuthenticator = override.LoginAuthenticator
	}
	if strings.TrimSpace(override.BasePath) != "" {
		base.BasePath = override.BasePath
	}
	if strings.TrimSpace(override.RefreshCookiePath) != "" {
		base.RefreshCookiePath = override.RefreshCookiePath
	}
	if strings.TrimSpace(override.CSRFCookiePath) != "" {
		base.CSRFCookiePath = override.CSRFCookiePath
	}
	if strings.TrimSpace(override.RefreshCookieName) != "" {
		base.RefreshCookieName = override.RefreshCookieName
	}
	if strings.TrimSpace(override.CSRFCookieName) != "" {
		base.CSRFCookieName = override.CSRFCookieName
	}
	if strings.TrimSpace(override.CSRFHeaderName) != "" {
		base.CSRFHeaderName = override.CSRFHeaderName
	}
	if len(override.TrustedProxies) > 0 {
		base.TrustedProxies = append([]netip.Prefix(nil), override.TrustedProxies...)
	}
	if override.MaxBodyBytes > 0 {
		base.MaxBodyBytes = override.MaxBodyBytes
	}
	if override.RateLimit != nil {
		base.RateLimit = mergeRateLimitOptions(base.RateLimit, override.RateLimit)
	}

	base.BasePath = normalizePath(base.BasePath)
	if strings.TrimSpace(base.RefreshCookiePath) == "" {
		base.RefreshCookiePath = base.BasePath
	} else {
		base.RefreshCookiePath = normalizePath(base.RefreshCookiePath)
	}
	base.CSRFCookiePath = normalizePath(base.CSRFCookiePath)
	return base
}

func mergeRateLimitOptions(base *RateLimitOptions, override *RateLimitOptions) *RateLimitOptions {
	if override == nil {
		return base
	}
	if base == nil {
		def := DefaultRateLimitOptions()
		base = &def
	}
	merged := *base
	merged.Disabled = override.Disabled
	if override.Requests > 0 {
		merged.Requests = override.Requests
	}
	if override.Window > 0 {
		merged.Window = override.Window
	}
	if override.IPv4PrefixBits > 0 {
		merged.IPv4PrefixBits = override.IPv4PrefixBits
	}
	if override.IPv6PrefixBits > 0 {
		merged.IPv6PrefixBits = override.IPv6PrefixBits
	}
	if len(override.TrustedProxies) > 0 {
		merged.TrustedProxies = append([]netip.Prefix(nil), override.TrustedProxies...)
	}
	if override.KeyFunc != nil {
		merged.KeyFunc = override.KeyFunc
	}
	return &merged
}

func normalizePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if len(p) > 1 && strings.HasSuffix(p, "/") {
		p = strings.TrimSuffix(p, "/")
	}
	return p
}

func joinPath(base, suffix string) string {
	base = normalizePath(base)
	if base == "/" {
		return normalizePath(suffix)
	}
	if !strings.HasPrefix(suffix, "/") {
		suffix = "/" + suffix
	}
	return base + suffix
}
