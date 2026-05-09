package middleware

import "net/http"

// LimitBody sets http.MaxBytesReader when maxBytes > 0.
// Call LimitBody before decoding the request body.
// LimitBody 在 maxBytes > 0 时设置 http.MaxBytesReader。
// 在解码请求体前调用 LimitBody。
func LimitBody(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if maxBytes > 0 {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}
