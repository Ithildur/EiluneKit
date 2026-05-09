package middleware

import (
	"net/http"

	"github.com/Ithildur/EiluneKit/http/response"

	"github.com/go-chi/chi/v5"
)

// MethodNotAllowedResponder returns a 405 handler that sets the Allow header.
// Call r.MethodNotAllowed(MethodNotAllowedResponder(r)).
// MethodNotAllowedResponder 返回会设置 Allow 头的 405 handler。
// 调用 r.MethodNotAllowed(MethodNotAllowedResponder(r))。
// Example / 示例:
//
//	r.MethodNotAllowed(middleware.MethodNotAllowedResponder(r))
func MethodNotAllowedResponder(routes chi.Routes) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		for _, method := range AllowedMethodsForRoute(routes, r) {
			w.Header().Add("Allow", method)
		}
		response.WriteJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	}
}
