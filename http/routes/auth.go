package routes

import (
	"context"
	"net/http"

	"github.com/Ithildur/EiluneKit/contextutil"
	"github.com/Ithildur/EiluneKit/http/response"
)

type authenticatedContextKey struct{}

// WithAuthenticated marks ctx as authenticated.
// Auth middleware should call it after credentials are accepted.
// WithAuthenticated 将 ctx 标记为已认证。
// 认证中间件应在凭据通过后调用它。
func WithAuthenticated(ctx context.Context) context.Context {
	return context.WithValue(contextutil.Require(ctx), authenticatedContextKey{}, true)
}

// Authenticated reports whether ctx was marked by WithAuthenticated.
// Authenticated 返回 ctx 是否已由 WithAuthenticated 标记。
func Authenticated(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	ok, _ := ctx.Value(authenticatedContextKey{}).(bool)
	return ok
}

func requireAuthenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !Authenticated(r.Context()) {
			response.WriteJSONError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		next.ServeHTTP(w, r)
	})
}
