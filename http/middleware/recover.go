package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
)

// RecoverOptions configures panic logging and the failure response.
// RecoverOptions 配置 panic 日志和失败响应。
type RecoverOptions struct {
	// Logger defaults to slog.Default at construction.
	// Logger 默认使用构造时的 slog.Default。
	Logger *slog.Logger
	// OnPanic writes the failure response before the response has started.
	// The pending Content-Length is cleared before the handler runs.
	// Nil uses http.Error with status 500 and no panic details.
	// OnPanic 在响应尚未开始时写入失败响应。
	// 调用前会清除待发送的 Content-Length。
	// nil 使用 http.Error 返回 500，不包含 panic 详情。
	OnPanic http.Handler
}

// Recover catches panics in the serving goroutine and logs the value and stack.
// After a response starts it aborts the request instead of writing another response.
// Hijacked connections are closed on panic. http.ErrAbortHandler is re-panicked without logging.
// Recover 捕获处理请求的 goroutine 中的 panic，记录其值和调用栈。
// 响应开始后会中止请求，不再写入第二个响应。
// panic 时关闭已接管的连接；http.ErrAbortHandler 不记录日志，直接再次抛出。
func Recover(opts RecoverOptions) func(http.Handler) http.Handler {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var state responseState
			writer := state.wrap(w)
			defer func() {
				value := recover()
				if value == nil {
					return
				}
				if state.conn != nil {
					_ = state.conn.Close()
				}
				if value == http.ErrAbortHandler {
					panic(value)
				}
				logger.ErrorContext(r.Context(), "http panic",
					"panic", value, "stack", string(debug.Stack()),
					"method", r.Method, "path", r.URL.Path,
					"request_id", RequestIDFromContext(r.Context()))
				if state.status != 0 || state.conn != nil {
					panic(http.ErrAbortHandler)
				}
				if opts.OnPanic != nil {
					w.Header().Del("Content-Length")
					opts.OnPanic.ServeHTTP(w, r)
					return
				}
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}()
			next.ServeHTTP(writer, r)
		})
	}
}
