// Package middleware provides reusable HTTP middleware.
// Package middleware 提供可复用 HTTP 中间件。
package middleware

import (
	"net/http"
	"strings"

	"github.com/Ithildur/EiluneKit/http/response"
)

// JSONOnly requires Content-Type: application/json for requests with a body.
// Call JSONOnly(next) around handlers that decode JSON.
// JSONOnly 要求带 body 的请求使用 Content-Type: application/json。
// 在解码 JSON 的 handler 外层调用 JSONOnly(next)。
// Example / 示例:
//
//	r.Post("/login", middleware.JSONOnly(http.HandlerFunc(handleLogin)))
func JSONOnly(next http.Handler) http.Handler {
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
