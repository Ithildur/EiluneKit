// Package middleware provides reusable HTTP middleware.
// Package middleware 提供可复用 HTTP 中间件。
package middleware

import (
	"net/http"
	"strings"

	"github.com/Ithildur/EiluneKit/http/response"
)

// RequireJSONBody requires Content-Type: application/json for requests with a body.
// Use routes.Use(RequireJSONBody) on routes that decode JSON.
// RequireJSONBody 要求带 body 的请求使用 Content-Type: application/json。
// 对会解码 JSON 的路由使用 routes.Use(RequireJSONBody)。
// Example / 示例:
//
//	r.Post("/login", "Login", routes.Func(loginHandler), routes.Use(middleware.RequireJSONBody))
func RequireJSONBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil && r.Body != http.NoBody {
			if r.ContentLength != 0 || len(r.TransferEncoding) > 0 {
				ct := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
				if i := strings.Index(ct, ";"); i >= 0 {
					ct = ct[:i]
				}
				if ct != "application/json" {
					response.WriteJSONError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "content-type must be application/json")
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}
