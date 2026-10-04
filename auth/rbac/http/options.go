package rbachttp

import (
	"net/netip"
	"slices"

	authhttp "github.com/Ithildur/EiluneKit/auth/http"
	corerbac "github.com/Ithildur/EiluneKit/auth/rbac"
)

const defaultAuthBasePath = "/auth"
const defaultMaxBodyBytes int64 = 1 << 20

// RateLimitOptions configures login rate limiting.
// RateLimitOptions 配置登录限流。
type RateLimitOptions = authhttp.RateLimitOptions

// Options configures NewHandler.
// Options 配置 NewHandler。
type Options struct {
	// BasePath is a route prefix. Nil defaults to "/auth"; new("") selects the root.
	// BasePath 是路由前缀；nil 默认使用 "/auth"，new("") 选择根目录。
	BasePath       *string
	MaxBodyBytes   int64
	TrustedProxies []netip.Prefix
	// ClientIPHeaders overrides clientip.Options.Headers for login lockout keys.
	// Nil uses the default order; an empty non-nil slice disables client IP headers.
	// ClientIPHeaders 覆盖登录锁定 key 所用的 clientip.Options.Headers。
	// Nil 使用默认顺序；非 nil 空切片禁用客户端 IP 头。
	ClientIPHeaders []string
	// RateLimit defaults to five login requests per minute per IP prefix.
	// Set Disabled to use an application-owned limiter instead.
	// RateLimit 默认按 IP 前缀每分钟允许五次登录请求。
	// 设置 Disabled 可改用应用自己的限流器。
	RateLimit  *RateLimitOptions
	RolePolicy corerbac.RolePolicy
}

// DefaultOptions returns default handler options.
// DefaultOptions 返回默认 handler 选项。
func DefaultOptions() Options {
	return Options{
		BasePath:     new(defaultAuthBasePath),
		MaxBodyBytes: defaultMaxBodyBytes,
		RateLimit:    new(authhttp.DefaultRateLimitOptions()),
	}
}

func applyOptions(base, override Options) Options {
	if override.BasePath != nil {
		base.BasePath = new(*override.BasePath)
	}
	if override.MaxBodyBytes > 0 {
		base.MaxBodyBytes = override.MaxBodyBytes
	}
	if len(override.TrustedProxies) > 0 {
		base.TrustedProxies = append([]netip.Prefix(nil), override.TrustedProxies...)
	}
	if override.ClientIPHeaders != nil {
		base.ClientIPHeaders = slices.Clone(override.ClientIPHeaders)
	}
	if override.RateLimit != nil {
		base.RateLimit = new(*override.RateLimit)
		base.RateLimit.TrustedProxies = slices.Clone(override.RateLimit.TrustedProxies)
		base.RateLimit.ClientIPHeaders = slices.Clone(override.RateLimit.ClientIPHeaders)
	}
	if override.RolePolicy != nil {
		base.RolePolicy = override.RolePolicy
	}
	return base
}
