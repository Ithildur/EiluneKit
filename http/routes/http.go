package routes

import (
	"cmp"
	"net/http"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"
)

// HandlerOptions configures the complete HTTP handler.
// HandlerOptions 配置完整的 HTTP handler。
type HandlerOptions struct {
	// Middleware runs in declaration order, including on unmatched requests.
	// Nil entries are ignored.
	// Middleware 按声明顺序执行，包括未匹配的请求；忽略 nil 项。
	Middleware []Middleware
	// NotFound handles unmatched paths. Nil uses http.NotFound.
	// NotFound 处理未匹配的路径；nil 使用 http.NotFound。
	NotFound http.Handler
	// MethodNotAllowed handles unsupported methods after Allow has been set.
	// Nil writes status 405 with no body.
	// MethodNotAllowed 在设置 Allow 后处理不支持的方法；nil 写入 405，不带响应体。
	MethodNotAllowed http.Handler
	// Unauthorized has the same contract as MountOptions.Unauthorized.
	// Unauthorized 与 MountOptions.Unauthorized 的契约相同。
	Unauthorized http.Handler
}

// NewHandler builds an HTTP handler from routes with their final paths.
// Middleware wraps the entire router; no middleware is installed implicitly.
// Later changes to route and middleware slices do not affect the handler.
// Validation errors and registration panics follow MountWithOptions.
// NewHandler 根据具有最终路径的路由构建 HTTP handler。
// 中间件包装整个路由器；不会隐式安装中间件。
// 后续修改路由和中间件切片不影响已构建的 handler。
// 校验错误和注册 panic 遵循 MountWithOptions。
func NewHandler(routeList []Route, opts HandlerOptions) (http.Handler, error) {
	mux := chi.NewRouter()
	if err := MountWithOptions(mux, "", routeList, MountOptions{Unauthorized: opts.Unauthorized}); err != nil {
		return nil, err
	}
	if opts.NotFound != nil {
		mux.NotFound(opts.NotFound.ServeHTTP)
	}

	methods := make([]string, 0, len(routeList))
	for _, route := range routeList {
		methods = append(methods, strings.ToUpper(strings.TrimSpace(route.Method)))
	}
	slices.Sort(methods)
	methods = slices.Compact(methods)
	mux.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Del("Allow")
		path := cmp.Or(chi.RouteContext(r.Context()).RoutePath, r.URL.RawPath, r.URL.Path, "/")
		// Probe with a separate context so failure handlers see the original request route.
		// 使用独立 context 探测，使失败 handler 仍能读取原始请求路由。
		ctx := chi.NewRouteContext()
		for _, method := range methods {
			ctx.Reset()
			if mux.Match(ctx, method, path) {
				w.Header().Add("Allow", method)
			}
		}
		if opts.MethodNotAllowed != nil {
			opts.MethodNotAllowed.ServeHTTP(w, r)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	return wrap(mux, opts.Middleware), nil
}
