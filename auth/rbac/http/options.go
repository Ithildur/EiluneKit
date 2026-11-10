package rbachttp

import (
	"net/netip"
	"slices"

	authhttp "github.com/Ithildur/EiluneKit/auth/http"
	"github.com/Ithildur/EiluneKit/auth/passkey"
	corerbac "github.com/Ithildur/EiluneKit/auth/rbac"
	captchahttp "github.com/Ithildur/EiluneKit/captcha/http"
)

const defaultAuthBasePath = "/auth"
const defaultMaxBodyBytes int64 = 1 << 20

// RateLimitOptions configures login rate limiting.
// RateLimitOptions 配置登录限流。
type RateLimitOptions = authhttp.RateLimitOptions

// Options configures NewHandler.
// Options 配置 NewHandler。
type Options struct {
	// Passkeys enables optional WebAuthn routes using RBAC session responses.
	// Passkeys 启用可选 WebAuthn 路由，使用 RBAC 会话响应。
	Passkeys *passkey.Service
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
	// RateLimit defaults to five requests per minute per IP prefix, separately for password login and passkey ceremonies.
	// Set Disabled to use an application-owned limiter instead.
	// RateLimit 默认按 IP 前缀对密码登录和通行密钥流程分别每分钟允许五次请求。
	// 设置 Disabled 可改用应用自己的限流器。
	RateLimit  *RateLimitOptions
	RolePolicy corerbac.RolePolicy
	// Captcha optionally verifies password login requests before credential authentication.
	// Captcha 可选地在凭据认证前验证密码登录请求。
	Captcha captchahttp.Options
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
	base.Passkeys = override.Passkeys
	base.Captcha = override.Captcha
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
