package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/Ithildur/EiluneKit/clientip"

	"github.com/go-chi/chi/v5/middleware"
)

// AccessLogOptions configures AccessLog.
// AccessLogOptions 配置 AccessLog。
type AccessLogOptions struct {
	Disabled bool
	Logger   *slog.Logger
	MinLevel slog.Level
	Skip     func(r *http.Request, status int) bool
	ClientIP clientip.Options
}

// AccessLog logs requests with method, path, status, and latency.
// Call r.Use(AccessLog(...)).
// AccessLog 记录请求的 method、path、status 和 latency。
// 调用 r.Use(AccessLog(...))。
//
// Example / 示例:
//
//	r.Use(middleware.AccessLog(middleware.AccessLogOptions{Logger: logger}))
func AccessLog(opts AccessLogOptions) func(http.Handler) http.Handler {
	if opts.Disabled || opts.Logger == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}
			if opts.Skip != nil && opts.Skip(r, status) {
				return
			}

			level := levelForStatus(status)
			if level < opts.MinLevel {
				return
			}

			attrs := []slog.Attr{
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", status),
				slog.Int64("latency_ms", time.Since(start).Milliseconds()),
			}
			if reqID := middleware.GetReqID(r.Context()); reqID != "" {
				attrs = append(attrs, slog.String("request_id", reqID))
			}
			if ip, ok := clientip.FromRequest(r, opts.ClientIP); ok {
				attrs = append(attrs, slog.String("remote_ip", ip.String()))
			}
			if ua := r.UserAgent(); ua != "" {
				attrs = append(attrs, slog.String("user_agent", ua))
			}

			opts.Logger.LogAttrs(r.Context(), level, "http_request", attrs...)
		})
	}
}

func levelForStatus(status int) slog.Level {
	switch {
	case status >= 500:
		return slog.LevelError
	case status >= 400:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}
