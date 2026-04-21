package middleware

import (
	"net/http"
	"strings"

	"github.com/Ithildur/EiluneKit/http/response"
)

// NotFoundHandler returns JSON 404 for /api paths and delegates the rest to staticHandler.
// Call r.NotFound(NotFoundHandler(staticHandler)).
// NotFoundHandler 对 /api 路径返回 JSON 404，其余交给 staticHandler。
// 调用 r.NotFound(NotFoundHandler(staticHandler))。
// Example / 示例:
//
//	staticHandler := http.FileServer(http.Dir("web/dist"))
//	r.NotFound(middleware.NotFoundHandler(staticHandler))
func NotFoundHandler(staticHandler http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL != nil && (r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/")) {
			response.WriteJSONError(w, http.StatusNotFound, "not_found", "resource not found")
			return
		}
		staticHandler.ServeHTTP(w, r)
	}
}
