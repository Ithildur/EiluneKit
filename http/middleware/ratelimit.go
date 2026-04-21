package middleware

import (
	"net/http"
	"net/netip"
	"time"

	"github.com/Ithildur/EiluneKit/clientip"
	"github.com/Ithildur/EiluneKit/http/response"

	"github.com/go-chi/httprate"
)

const (
	defaultRateLimitRequests = 60
	defaultRateLimitWindow   = time.Minute
)

// RateLimitOptions configures RateLimit.
// RateLimitOptions 配置 RateLimit。
type RateLimitOptions struct {
	Requests int
	Window   time.Duration
	KeyFunc  httprate.KeyFunc
	OnLimit  func(http.ResponseWriter, *http.Request)
}

// RateLimitKeyOptions configures RateLimitKeyByIP.
// RateLimitKeyOptions 配置 RateLimitKeyByIP。
type RateLimitKeyOptions struct {
	TrustedProxies []netip.Prefix
}

// RateLimit returns a rate-limit middleware.
// Call r.Use(RateLimit(opts)).
// RateLimit 返回限流中间件。
// 调用 r.Use(RateLimit(opts))。
// Example / 示例:
//
//	r.Use(middleware.RateLimit(middleware.RateLimitOptions{Requests: 100, Window: time.Minute}))
func RateLimit(opts RateLimitOptions) func(http.Handler) http.Handler {
	effective := opts
	if effective.Requests <= 0 {
		effective.Requests = defaultRateLimitRequests
	}
	if effective.Window <= 0 {
		effective.Window = defaultRateLimitWindow
	}
	if effective.KeyFunc == nil {
		effective.KeyFunc = RateLimitKeyByIP(24, 40, RateLimitKeyOptions{})
	}
	if effective.OnLimit == nil {
		effective.OnLimit = func(w http.ResponseWriter, r *http.Request) {
			response.WriteJSONError(w, http.StatusTooManyRequests, "rate_limited", "too many requests")
		}
	}

	return httprate.Limit(
		effective.Requests,
		effective.Window,
		httprate.WithKeyFuncs(effective.KeyFunc),
		httprate.WithLimitHandler(effective.OnLimit),
	)
}

// RateLimitKeyByIP builds a rate-limit key from the client IP prefix.
// Forwarded headers are used only when TrustedProxies is set.
// RateLimitKeyByIP 根据客户端 IP 前缀构造限流键。
// 只有设置 TrustedProxies 时才会使用转发头。
func RateLimitKeyByIP(ipv4PrefixBits, ipv6PrefixBits int, opts RateLimitKeyOptions) httprate.KeyFunc {
	v4 := normalizeRateLimitPrefixBits(ipv4PrefixBits, 32)
	v6 := normalizeRateLimitPrefixBits(ipv6PrefixBits, 128)
	clientIPOpts := clientip.Options{
		TrustedProxies: append([]netip.Prefix(nil), opts.TrustedProxies...),
	}

	return func(r *http.Request) (string, error) {
		ip, ok := clientip.FromRequest(r, clientIPOpts)
		if !ok {
			return "unknown", nil
		}
		if ip.Is4() {
			p := netip.PrefixFrom(ip, v4).Masked()
			return "v4:" + p.String(), nil
		}
		p := netip.PrefixFrom(ip, v6).Masked()
		return "v6:" + p.String(), nil
	}
}

func normalizeRateLimitPrefixBits(bits, max int) int {
	if bits <= 0 || bits > max {
		return max
	}
	return bits
}
