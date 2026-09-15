package response

import "net/http"

// NotFound returns a JSON 404 handler for any request path.
// NotFound 返回适用于任意请求路径的 JSON 404 handler。
func NotFound() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		WriteJSONError(w, http.StatusNotFound, "not_found", "resource not found")
	}
}

// MethodNotAllowed returns a JSON 405 handler. The router must set Allow.
// MethodNotAllowed 返回 JSON 405 handler；Allow 必须由路由器设置。
func MethodNotAllowed() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		WriteJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	}
}

// Unauthorized returns a JSON 401 handler. Authentication headers belong to the application.
// Unauthorized 返回 JSON 401 handler；认证响应头由应用设置。
func Unauthorized() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		WriteJSONError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
	}
}

// InternalServerError returns a JSON 500 handler without internal error details.
// InternalServerError 返回不含内部错误详情的 JSON 500 handler。
func InternalServerError() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		WriteJSONError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
