package middleware

import (
	"net/http"
	"slices"
	"time"

	"github.com/rs/cors"
)

// CORSOptions configures cross-origin access. No origins are allowed by default.
// CORSOptions 配置跨域访问；默认不允许任何来源。
type CORSOptions struct {
	// AllowedOrigins accepts exact origins or patterns with one '*'.
	// AllowedOrigins 接受精确来源或含一个 '*' 的模式。
	AllowedOrigins []string
	// AllowedMethods defaults to GET, HEAD, and POST. List methods explicitly.
	// AllowedMethods 默认允许 GET、HEAD 和 POST；请显式列出方法。
	AllowedMethods []string
	// AllowedHeaders defaults to Accept, Content-Type, and X-Requested-With; '*' allows all.
	// AllowedHeaders 默认允许 Accept、Content-Type 和 X-Requested-With；'*' 允许全部。
	AllowedHeaders []string
	ExposedHeaders []string
	// AllowCredentials permits credentials for the configured origins.
	// AllowCredentials 允许配置的来源携带凭证。
	AllowCredentials bool
	// MaxAge is the preflight cache duration, truncated to whole seconds.
	// Zero leaves the browser default unchanged.
	// MaxAge 是预检缓存时长，截断为整秒；零值保留浏览器默认值。
	MaxAge time.Duration
}

// CORS handles preflight requests before routing and adds CORS response headers.
// Rejected requests receive no access permission headers; this is not authentication.
// Panics for negative MaxAge or '*' origins combined with credentials.
// CORS 在路由匹配前处理预检请求，并添加 CORS 响应头。
// 被拒绝的请求不获得访问许可响应头；这不是认证机制。
// MaxAge 为负数或将 '*' 来源与凭证组合使用时 panic。
func CORS(opts CORSOptions) func(http.Handler) http.Handler {
	if opts.MaxAge < 0 {
		panic("middleware: CORS MaxAge must not be negative")
	}
	if opts.AllowCredentials && slices.Contains(opts.AllowedOrigins, "*") {
		panic("middleware: CORS wildcard origin cannot allow credentials")
	}
	config := cors.Options{
		AllowedOrigins:   slices.Clone(opts.AllowedOrigins),
		AllowedMethods:   slices.Clone(opts.AllowedMethods),
		AllowedHeaders:   slices.Clone(opts.AllowedHeaders),
		ExposedHeaders:   slices.Clone(opts.ExposedHeaders),
		AllowCredentials: opts.AllowCredentials,
		MaxAge:           int(opts.MaxAge / time.Second),
	}
	if len(config.AllowedOrigins) == 0 {
		config.AllowOriginFunc = func(string) bool { return false }
	}
	return cors.New(config).Handler
}
