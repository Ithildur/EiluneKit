package rbac

import (
	"net/netip"
	"strings"

	corerbac "github.com/Ithildur/EiluneKit/auth/rbac"
)

const defaultAuthBasePath = "/auth"
const defaultMaxBodyBytes int64 = 1 << 20

// Options configures NewHandler.
// Options 配置 NewHandler。
type Options struct {
	BasePath       string
	MaxBodyBytes   int64
	TrustedProxies []netip.Prefix
	RolePolicy     corerbac.RolePolicy
}

// DefaultOptions returns default handler options.
// DefaultOptions 返回默认 handler 选项。
func DefaultOptions() Options {
	return Options{
		BasePath:     defaultAuthBasePath,
		MaxBodyBytes: defaultMaxBodyBytes,
	}
}

func applyOptions(base, override Options) Options {
	if strings.TrimSpace(override.BasePath) != "" {
		base.BasePath = override.BasePath
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
	base.BasePath = normalizePath(base.BasePath)
	return base
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
