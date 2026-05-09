// Package session provides cookie helpers for auth flows.
// Package session 提供认证流程的 cookie 辅助函数。
//
// Usage (Double-submit CSRF) / 用法（双提交 CSRF）:
//
//	csrf := uuid.NewString()
//	SetCSRFCookie(w, csrf, exp, CookieConfig{Name: DefaultCSRFCookieName, Path: "/"})
//	// Client sends header: X-CSRF-Token: <csrf>
//	ok := ValidateDoubleSubmit(r, DefaultCSRFCookieName, DefaultCSRFHeaderName)
package session

import (
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/Ithildur/EiluneKit/clientip"
)

// CookieConfig configures auth cookies.
// CookieConfig 配置认证 cookie。
type CookieConfig struct {
	Name        string
	Path        string
	Domain      string
	Secure      bool
	SameSite    http.SameSite
	SessionOnly bool
}

const (
	DefaultRefreshCookieName = "refresh"
	DefaultCSRFCookieName    = "csrf"
	DefaultCSRFHeaderName    = "X-CSRF-Token"
)

// CookieTrustOptions configures forwarded-proto trust.
// CookieTrustOptions 配置转发协议头信任边界。
type CookieTrustOptions struct {
	TrustedProxies []netip.Prefix
}

// SetRefreshCookie writes the refresh token cookie.
// SetRefreshCookie 写入 refresh token cookie。
func SetRefreshCookie(w http.ResponseWriter, token string, exp time.Time, cfg CookieConfig) {
	setCookie(w, token, exp, cfg, true)
}

// SetCSRFCookie writes the CSRF token cookie.
// SetCSRFCookie 写入 CSRF token cookie。
func SetCSRFCookie(w http.ResponseWriter, token string, exp time.Time, cfg CookieConfig) {
	setCookie(w, token, exp, cfg, false)
}

// ClearCookie removes a cookie.
// ClearCookie 删除 cookie。
func ClearCookie(w http.ResponseWriter, cfg CookieConfig) {
	c := http.Cookie{
		Name:     cfg.Name,
		Path:     cookiePath(cfg.Path),
		Domain:   cfg.Domain,
		MaxAge:   -1,
		Secure:   cfg.Secure,
		SameSite: cfg.SameSite,
		HttpOnly: false,
	}
	http.SetCookie(w, &c)
}

// ReadCookie returns the cookie value.
// Missing cookies return an empty string.
// ReadCookie 返回 cookie 值。
// 缺失 cookie 时返回空字符串。
func ReadCookie(r *http.Request, name string) string {
	if r == nil || name == "" {
		return ""
	}
	c, err := r.Cookie(name)
	if err != nil || c == nil {
		return ""
	}
	return c.Value
}

// ValidateDoubleSubmit compares the CSRF cookie and header.
// ValidateDoubleSubmit 比较 CSRF cookie 与 header。
func ValidateDoubleSubmit(r *http.Request, cookieName, headerName string) bool {
	if r == nil {
		return false
	}
	cookie := ReadCookie(r, cookieName)
	if cookie == "" {
		return false
	}
	header := strings.TrimSpace(r.Header.Get(headerName))
	if header == "" {
		return false
	}
	return header == cookie
}

// DefaultCookieConfig derives Secure and SameSite from r.
// TLS is always trusted. X-Forwarded-Proto is trusted only for TrustedProxies.
// DefaultCookieConfig 从 r 推导 Secure 与 SameSite。
// TLS 总是可信；X-Forwarded-Proto 仅对 TrustedProxies 生效。
func DefaultCookieConfig(r *http.Request, opts CookieTrustOptions) CookieConfig {
	secure := false
	if r != nil && r.TLS != nil {
		secure = true
	}
	if !secure && r != nil {
		if proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); strings.EqualFold(proto, "https") && isTrustedProxyPeer(r, opts.TrustedProxies) {
			secure = true
		}
	}

	sameSite := http.SameSiteLaxMode
	if secure {
		sameSite = http.SameSiteNoneMode
	}

	return CookieConfig{
		Secure:   secure,
		SameSite: sameSite,
	}
}

func isTrustedProxyPeer(r *http.Request, trusted []netip.Prefix) bool {
	if r == nil || len(trusted) == 0 {
		return false
	}
	ip, ok := clientip.FromRemote(r.RemoteAddr)
	if !ok {
		return false
	}
	for _, prefix := range trusted {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}

func setCookie(w http.ResponseWriter, token string, exp time.Time, cfg CookieConfig, httpOnly bool) {
	if w == nil || cfg.Name == "" {
		return
	}
	c := http.Cookie{
		Name:     cfg.Name,
		Value:    token,
		Path:     cookiePath(cfg.Path),
		Domain:   cfg.Domain,
		Secure:   cfg.Secure,
		SameSite: cfg.SameSite,
		HttpOnly: httpOnly,
	}
	if !cfg.SessionOnly {
		maxAge := int(time.Until(exp).Seconds())
		if maxAge < 0 {
			maxAge = 0
		}
		c.Expires = exp
		c.MaxAge = maxAge
	}
	http.SetCookie(w, &c)
}

func cookiePath(path string) string {
	if path == "" {
		return "/"
	}
	if strings.HasPrefix(path, "/") {
		return path
	}
	return "/" + path
}
