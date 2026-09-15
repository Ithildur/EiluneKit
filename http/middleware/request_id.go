package middleware

import (
	"context"
	"net/http"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// RequestID puts the incoming X-Request-Id, or a generated ID, in the context.
// It uses chi's request ID context, shared with AccessLog, and does not set a response header.
// RequestID 将传入的 X-Request-Id 或生成的 ID 放入 context。
// 使用与 AccessLog 共享的 chi 请求 ID context，不设置响应头。
func RequestID(next http.Handler) http.Handler {
	return chimw.RequestID(next)
}

// RequestIDFromContext returns the request ID, or "" when absent or ctx is nil.
// RequestIDFromContext 返回请求 ID；不存在或 ctx 为 nil 时返回空字符串。
func RequestIDFromContext(ctx context.Context) string {
	return chimw.GetReqID(ctx)
}
