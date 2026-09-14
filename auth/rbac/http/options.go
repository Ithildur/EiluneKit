package rbachttp

import (
	"net/netip"

	corerbac "github.com/Ithildur/EiluneKit/auth/rbac"
)

const defaultAuthBasePath = "auth"
const defaultMaxBodyBytes int64 = 1 << 20

// Options configures NewHandler.
// Options 配置 NewHandler。
type Options struct {
	// BasePath is a relative mount directory. Nil defaults to "auth"; new("") selects the root.
	// BasePath 是相对挂载目录；nil 默认使用 "auth"，new("") 选择根目录。
	BasePath       *string
	MaxBodyBytes   int64
	TrustedProxies []netip.Prefix
	RolePolicy     corerbac.RolePolicy
}

// DefaultOptions returns default handler options.
// DefaultOptions 返回默认 handler 选项。
func DefaultOptions() Options {
	return Options{
		BasePath:     new(defaultAuthBasePath),
		MaxBodyBytes: defaultMaxBodyBytes,
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
	if override.RolePolicy != nil {
		base.RolePolicy = override.RolePolicy
	}
	return base
}
