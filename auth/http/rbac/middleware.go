package rbac

import (
	"context"
	"errors"
	"net/http"
	"strings"

	authcore "github.com/Ithildur/EiluneKit/auth"
	authjwt "github.com/Ithildur/EiluneKit/auth/jwt"
	corerbac "github.com/Ithildur/EiluneKit/auth/rbac"
	"github.com/Ithildur/EiluneKit/http/response"
	"github.com/Ithildur/EiluneKit/http/routes"
)

// Authenticator authenticates bearer tokens.
// Authenticator 校验 bearer token。
type Authenticator interface {
	AuthenticateBearer(ctx context.Context, token string) (authcore.Principal, bool, error)
}

// Middleware builds RBAC route middleware.
// Middleware 构造 RBAC 路由中间件。
type Middleware struct {
	Auth  Authenticator
	Roles corerbac.RolePolicy
}

// NewMiddleware returns RBAC middleware helpers.
// NewMiddleware 返回 RBAC middleware 辅助工具。
func NewMiddleware(auth Authenticator, roles corerbac.RolePolicy) Middleware {
	if roles == nil {
		roles = corerbac.ExactRolePolicy{}
	}
	return Middleware{Auth: auth, Roles: roles}
}

// RequireAuth requires any authenticated principal.
// RequireAuth 要求任意已认证主体。
func (m Middleware) RequireAuth() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, req, ok := m.principal(w, r)
			if !ok {
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}

// RequireRole requires a role allowed by the configured RolePolicy.
// RequireRole 要求配置的 RolePolicy 允许该角色。
func (m Middleware) RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			required := strings.TrimSpace(role)
			if required == "" {
				writeAuthMisconfigured(w)
				return
			}
			p, req, ok := m.principal(w, r)
			if !ok {
				return
			}
			if !m.rolePolicy().Allows(p.Role, required) {
				writeForbidden(w)
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}

// RequireAnyRole requires at least one allowed role.
// RequireAnyRole 要求至少一个被允许的角色。
func (m Middleware) RequireAnyRole(roles ...string) func(http.Handler) http.Handler {
	required := append([]string(nil), roles...)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cleaned := cleanStrings(append([]string(nil), required...))
			if len(cleaned) == 0 {
				writeAuthMisconfigured(w)
				return
			}
			p, req, ok := m.principal(w, r)
			if !ok {
				return
			}
			policy := m.rolePolicy()
			for _, role := range cleaned {
				if policy.Allows(p.Role, role) {
					next.ServeHTTP(w, req)
					return
				}
			}
			writeForbidden(w)
		})
	}
}

// RequireScope requires a scope.
// RequireScope 要求指定 scope。
func (m Middleware) RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			required := strings.TrimSpace(scope)
			if required == "" {
				writeAuthMisconfigured(w)
				return
			}
			p, req, ok := m.principal(w, r)
			if !ok {
				return
			}
			if !p.HasScope(required) {
				writeForbidden(w)
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}

// RequireRoleAndScope requires both a role and a scope.
// RequireRoleAndScope 同时要求角色与 scope。
func (m Middleware) RequireRoleAndScope(role, scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requiredRole := strings.TrimSpace(role)
			requiredScope := strings.TrimSpace(scope)
			if requiredRole == "" || requiredScope == "" {
				writeAuthMisconfigured(w)
				return
			}
			p, req, ok := m.principal(w, r)
			if !ok {
				return
			}
			if !m.rolePolicy().Allows(p.Role, requiredRole) || !p.HasScope(requiredScope) {
				writeForbidden(w)
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}

func (m Middleware) principal(w http.ResponseWriter, r *http.Request) (authcore.Principal, *http.Request, bool) {
	if p, ok := authcore.PrincipalFromContext(r.Context()); ok {
		return p, r.WithContext(routes.WithAuthenticated(r.Context())), true
	}
	if m.Auth == nil {
		writeAuthMisconfigured(w)
		return authcore.Principal{}, nil, false
	}
	token, ok := parseBearerHeader(r.Header.Get("Authorization"))
	if !ok {
		response.WriteJSONError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid bearer token")
		return authcore.Principal{}, nil, false
	}
	p, ok, err := m.Auth.AuthenticateBearer(r.Context(), token)
	if err != nil {
		writeAuthFailure(w, err)
		return authcore.Principal{}, nil, false
	}
	if !ok {
		response.WriteJSONError(w, http.StatusUnauthorized, "unauthorized", "token invalid or expired")
		return authcore.Principal{}, nil, false
	}
	ctx := authcore.WithPrincipal(r.Context(), p)
	ctx = routes.WithAuthenticated(ctx)
	return p, r.WithContext(ctx), true
}

func (m Middleware) rolePolicy() corerbac.RolePolicy {
	if m.Roles == nil {
		return corerbac.ExactRolePolicy{}
	}
	return m.Roles
}

func parseBearerHeader(header string) (string, bool) {
	parts := strings.Fields(strings.TrimSpace(header))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

func cleanStrings(values []string) []string {
	out := values[:0]
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func writeAuthFailure(w http.ResponseWriter, err error) {
	switch {
	case err == nil:
		response.WriteJSONError(w, http.StatusInternalServerError, "auth_error", "auth failed")
	case errors.Is(err, corerbac.ErrLoginLocked):
		response.WriteJSONError(w, http.StatusTooManyRequests, "login_locked", "login locked")
	case errors.Is(err, authjwt.ErrStoreUnavailable):
		response.WriteJSONError(w, http.StatusServiceUnavailable, "auth_unavailable", "auth is unavailable")
	case errors.Is(err, authjwt.ErrUnauthorized):
		response.WriteJSONError(w, http.StatusUnauthorized, "unauthorized", "token invalid or expired")
	case errors.Is(err, corerbac.ErrEventFailed):
		response.WriteJSONError(w, http.StatusInternalServerError, "auth_event_error", "auth event failed")
	case isAuthMisconfigured(err):
		writeAuthMisconfigured(w)
	default:
		response.WriteJSONError(w, http.StatusInternalServerError, "auth_error", "auth failed")
	}
}

func isAuthMisconfigured(err error) bool {
	return errors.Is(err, corerbac.ErrServiceMisconfigured) ||
		errors.Is(err, corerbac.ErrUserStoreMissing) ||
		errors.Is(err, corerbac.ErrPasswordVerifierMissing) ||
		errors.Is(err, corerbac.ErrTokenManagerMissing) ||
		errors.Is(err, corerbac.ErrAPITokenStoreMissing) ||
		errors.Is(err, corerbac.ErrUserIDRequired) ||
		errors.Is(err, authjwt.ErrManagerMisconfigured) ||
		errors.Is(err, authjwt.ErrUserIDRequired) ||
		errors.Is(err, authjwt.ErrSessionIDRequired)
}

func writeAuthMisconfigured(w http.ResponseWriter) {
	response.WriteJSONError(w, http.StatusInternalServerError, "auth_misconfigured", "auth is misconfigured")
}

func writeForbidden(w http.ResponseWriter) {
	response.WriteJSONError(w, http.StatusForbidden, "forbidden", "forbidden")
}
