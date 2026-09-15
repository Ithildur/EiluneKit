package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/Ithildur/EiluneKit/clientip"
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
// Aborted requests include aborted=true and status=0 when no final status was observed.
// Panics propagate unchanged. Skip and MinLevel also apply to aborted requests.
// AccessLog 记录请求的 method、path、status 和 latency。
// 中止请求包含 aborted=true；未观察到最终状态时 status=0。
// panic 原样传播；Skip 和 MinLevel 同样适用于中止请求。
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
			var state responseState
			completed := false
			defer func() {
				status := state.status
				if status == 0 && completed {
					status = http.StatusOK
				}
				if opts.Skip != nil && opts.Skip(r, status) {
					return
				}
				level := levelForStatus(status)
				if !completed {
					level = slog.LevelError
				}
				if level < opts.MinLevel || !opts.Logger.Enabled(r.Context(), level) {
					return
				}
				attrs := []slog.Attr{
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.Int("status", status),
					slog.Int64("latency_ms", time.Since(start).Milliseconds()),
				}
				if !completed {
					attrs = append(attrs, slog.Bool("aborted", true))
				}
				if reqID := RequestIDFromContext(r.Context()); reqID != "" {
					attrs = append(attrs, slog.String("request_id", reqID))
				}
				if ip, ok := clientip.FromRequest(r, opts.ClientIP); ok {
					attrs = append(attrs, slog.String("remote_ip", ip.String()))
				}
				if ua := r.UserAgent(); ua != "" {
					attrs = append(attrs, slog.String("user_agent", ua))
				}
				opts.Logger.LogAttrs(r.Context(), level, "http_request", attrs...)
			}()
			next.ServeHTTP(state.wrap(w), r)
			completed = true
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
