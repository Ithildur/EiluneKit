package middleware

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

var allowMethodOrder = []string{
	http.MethodGet,
	http.MethodHead,
	http.MethodPost,
	http.MethodPut,
	http.MethodPatch,
	http.MethodDelete,
	http.MethodOptions,
	http.MethodTrace,
	http.MethodConnect,
}

// AllowedMethodsForRoute returns allowed methods for the matched route.
// AllowedMethodsForRoute 返回匹配路由允许的 HTTP 方法。
func AllowedMethodsForRoute(routes chi.Routes, r *http.Request) []string {
	if r == nil || routes == nil {
		return nil
	}
	rctx := chi.RouteContext(r.Context())
	if rctx == nil {
		return nil
	}
	routePath := rctx.RoutePath
	if routePath == "" && r.URL != nil {
		if r.URL.RawPath != "" {
			routePath = r.URL.RawPath
		} else {
			routePath = r.URL.Path
		}
	}
	if routePath == "" {
		routePath = "/"
	}

	allowed := make([]string, 0, len(allowMethodOrder))
	for _, method := range allowMethodOrder {
		tctx := chi.NewRouteContext()
		if routes.Match(tctx, method, routePath) {
			allowed = append(allowed, method)
		}
	}
	return allowed
}
