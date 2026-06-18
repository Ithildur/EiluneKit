package auth

import (
	"context"

	"github.com/Ithildur/EiluneKit/contextutil"
)

// PrincipalKind describes the authenticated subject type.
// PrincipalKind 描述已认证主体类型。
type PrincipalKind string

const (
	// PrincipalKindUser identifies a user session.
	// PrincipalKindUser 表示用户 session。
	PrincipalKindUser PrincipalKind = "user"
	// PrincipalKindAPIToken identifies an API token.
	// PrincipalKindAPIToken 表示 API token。
	PrincipalKindAPIToken PrincipalKind = "api_token"
)

// Principal is the authenticated subject attached to request contexts.
// Scopes is owned by the Principal value and is copied when stored.
// Principal 是写入请求 context 的已认证主体。
// Scopes 归 Principal 值所有，写入时会复制。
type Principal struct {
	Subject  string        `json:"subject"`
	Username string        `json:"username,omitempty"`
	Role     string        `json:"role,omitempty"`
	Scopes   []string      `json:"scopes,omitempty"`
	Kind     PrincipalKind `json:"kind"`
}

type principalContextKey struct{}

// WithPrincipal stores p on ctx.
// WithPrincipal 将 p 写入 ctx。
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	p.Scopes = append([]string(nil), p.Scopes...)
	return context.WithValue(contextutil.Require(ctx), principalContextKey{}, p)
}

// PrincipalFromContext returns the Principal stored by WithPrincipal.
// PrincipalFromContext 返回 WithPrincipal 写入的 Principal。
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	if ctx == nil {
		return Principal{}, false
	}
	p, ok := ctx.Value(principalContextKey{}).(Principal)
	if !ok {
		return Principal{}, false
	}
	p.Scopes = append([]string(nil), p.Scopes...)
	return p, true
}

// HasScope reports whether p contains scope.
// HasScope 返回 p 是否包含 scope。
func (p Principal) HasScope(scope string) bool {
	for _, s := range p.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}
