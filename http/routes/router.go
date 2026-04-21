package routes

import "github.com/go-chi/chi/v5"

// Router collects routes under prefixes.
// Build routes at startup.
// Nil Router is invalid and panics on use.
// Router 收集并组织带前缀的路由。
// 请在启动阶段构建路由。
// Nil Router 无效，使用时会 panic。
type Router struct {
	routes []Route
}

// NewRouter returns an empty Router.
// NewRouter 返回空 Router。
func NewRouter() *Router {
	return &Router{routes: make([]Route, 0)}
}

// Include adds routes under prefix.
// Include 在 prefix 下添加路由。
func (r *Router) Include(prefix string, routes []Route) {
	r = requireRouter(r)
	if len(routes) == 0 {
		return
	}
	r.routes = append(r.routes, ownedRoutes(prefix, routes)...)
}

// Routes returns a copy of the routes.
// Routes 返回路由副本。
func (r *Router) Routes() []Route {
	r = requireRouter(r)
	return cloneRoutes(r.routes)
}

// Mount registers the routes on router.
// Mount 在 router 上注册路由。
func (r *Router) Mount(router chi.Router, prefix string, opts ...MountOption) error {
	r = requireRouter(r)
	return Mount(router, prefix, r.routes, opts...)
}

// ExportJSON exports route metadata as JSON.
// ExportJSON 导出 JSON 路由元数据。
func (r *Router) ExportJSON() ([]byte, error) {
	r = requireRouter(r)
	return ExportJSON(r.routes)
}

// ExportMarkdown exports route metadata as Markdown.
// ExportMarkdown 导出 Markdown 路由表。
func (r *Router) ExportMarkdown() (string, error) {
	r = requireRouter(r)
	return ExportMarkdown(r.routes)
}

// WithTags returns routes with tags appended.
// WithTags 返回追加 tags 后的路由副本。
func WithTags(routes []Route, tags ...string) []Route {
	if len(routes) == 0 {
		return nil
	}

	out := cloneRoutes(routes)
	if len(tags) == 0 {
		return out
	}

	for i := range out {
		out[i].Tags = append(out[i].Tags, tags...)
	}
	return out
}

// WithMiddleware returns routes with prepended middleware.
// WithMiddleware 返回预置最外层中间件后的路由副本。
func WithMiddleware(routes []Route, mws ...Middleware) []Route {
	if len(routes) == 0 {
		return nil
	}

	out := cloneRoutes(routes)
	if len(mws) == 0 {
		return out
	}

	for i := range out {
		merged := append([]Middleware(nil), mws...)
		out[i].Middleware = append(merged, out[i].Middleware...)
	}
	return out
}

func ownedRoutes(prefix string, routes []Route) []Route {
	if len(routes) == 0 {
		return nil
	}

	out := cloneRoutes(routes)
	p := cleanPrefix(prefix)
	if p == "" {
		return out
	}

	for i := range out {
		out[i].Path = joinPath(p, out[i].Path)
	}
	return out
}

func requireRouter(r *Router) *Router {
	if r == nil {
		panic("routes: nil Router")
	}
	return r
}
