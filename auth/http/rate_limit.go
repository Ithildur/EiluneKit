package authhttp

import (
	stdhttp "net/http"
	"net/netip"
	"time"

	"github.com/Ithildur/EiluneKit/http/middleware"

	"github.com/go-chi/httprate"
)

const (
	defaultRateLimitRequests = 5
	defaultRateLimitWindow   = time.Minute
)

// RateLimitOptions configures LoginRateLimit.
// RateLimitOptions 配置 LoginRateLimit。
type RateLimitOptions struct {
	Disabled       bool
	Requests       int
	Window         time.Duration
	IPv4PrefixBits int
	IPv6PrefixBits int
	TrustedProxies []netip.Prefix
	KeyFunc        httprate.KeyFunc
}

// DefaultRateLimitOptions returns the default login rate limit options.
// DefaultRateLimitOptions 返回默认登录限流选项。
func DefaultRateLimitOptions() RateLimitOptions {
	return RateLimitOptions{
		Disabled:       false,
		Requests:       defaultRateLimitRequests,
		Window:         defaultRateLimitWindow,
		IPv4PrefixBits: 24,
		IPv6PrefixBits: 40,
	}
}

// LoginRateLimit returns a login rate-limit middleware.
// Disabled or nil options return nil.
// LoginRateLimit 返回登录限流中间件。
// 传入 nil 或 Disabled 时返回 nil。
func LoginRateLimit(opts *RateLimitOptions) func(stdhttp.Handler) stdhttp.Handler {
	if opts == nil {
		return nil
	}
	effective := *opts
	if effective.Disabled {
		return nil
	}
	if effective.Requests <= 0 {
		effective.Requests = defaultRateLimitRequests
	}
	if effective.Window <= 0 {
		effective.Window = defaultRateLimitWindow
	}
	if effective.IPv4PrefixBits <= 0 {
		effective.IPv4PrefixBits = 24
	}
	if effective.IPv6PrefixBits <= 0 {
		effective.IPv6PrefixBits = 40
	}
	if effective.KeyFunc == nil {
		effective.KeyFunc = middleware.RateLimitKeyByIP(
			effective.IPv4PrefixBits,
			effective.IPv6PrefixBits,
			middleware.RateLimitKeyOptions{
				TrustedProxies: append([]netip.Prefix(nil), effective.TrustedProxies...),
			},
		)
	}
	return middleware.RateLimit(middleware.RateLimitOptions{
		Requests: effective.Requests,
		Window:   effective.Window,
		KeyFunc:  effective.KeyFunc,
	})
}
