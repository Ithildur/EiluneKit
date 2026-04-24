package routes

import (
	"net/http"
	"reflect"

	"github.com/go-chi/chi/v5"
)

// Blueprint groups routes and route defaults.
// Call NewBlueprint, add routes with Get/Post/... , then mount with Mount or MountAt.
// Nil Blueprint is invalid and panics on use.
// Blueprint 分组管理路由及默认配置。
// 调用 NewBlueprint，使用 Get/Post/... 添加路由，再通过 Mount 或 MountAt 挂载。
// Nil Blueprint 无效，使用时会 panic。
type Blueprint struct {
	routes     []Route
	tags       []string
	auth       AuthPolicy
	hasAuth    bool
	middleware []Middleware
}

// BlueprintOption configures defaults for routes added to a Blueprint.
// BlueprintOption 配置 Blueprint 中新增路由的默认值。
type BlueprintOption func(*blueprintConfig)

type blueprintConfig struct {
	tags       []string
	auth       AuthPolicy
	hasAuth    bool
	middleware []Middleware
}

// DefaultTags prepends tags to routes added to a Blueprint.
// DefaultTags 为 Blueprint 中新增路由预置 tags。
func DefaultTags(tags ...string) BlueprintOption {
	return func(c *blueprintConfig) {
		c.tags = append(c.tags, tags...)
	}
}

// DefaultAuth sets the default auth policy.
// DefaultAuth 设置默认认证策略。
func DefaultAuth(policy AuthPolicy) BlueprintOption {
	return func(c *blueprintConfig) {
		c.auth = policy
		c.hasAuth = true
	}
}

// DefaultMiddleware prepends middleware to routes added to a Blueprint.
// DefaultMiddleware 为 Blueprint 中新增路由预置最外层中间件。
func DefaultMiddleware(mw ...Middleware) BlueprintOption {
	return func(c *blueprintConfig) {
		c.middleware = append(c.middleware, mw...)
	}
}

// RouteOption modifies a route before it is added.
// RouteOption 在路由加入前修改路由。
type RouteOption func(*Route)

type handlerSpec struct {
	handler http.Handler
}

// Tags appends Route.Tags.
// Tags 追加 Route.Tags。
func Tags(tags ...string) RouteOption {
	return func(r *Route) {
		r.Tags = append(r.Tags, tags...)
	}
}

// Auth sets Route.Auth.
// Auth 设置 Route.Auth。
func Auth(policy AuthPolicy) RouteOption {
	return func(r *Route) {
		r.Auth = policy
	}
}

// Use appends route middleware.
// Use 追加路由中间件。
func Use(mw ...Middleware) RouteOption {
	return func(r *Route) {
		r.Middleware = append(r.Middleware, mw...)
	}
}

// Handler builds the required Blueprint handler from an existing http.Handler.
// Prefer Func for plain http.HandlerFunc values and methods.
// Panics if h is nil, including typed-nil handlers.
// Handler 使用现成的 http.Handler 构造 Blueprint 必填 handler。
// 普通 http.HandlerFunc 或方法值优先用 Func。
// h 为 nil 时会 panic，包括 typed-nil handler。
func Handler(h http.Handler) handlerSpec {
	return handlerSpec{handler: mustHandler(h)}
}

// Func builds the required Blueprint handler from an http.HandlerFunc.
// This is the preferred option for plain handler functions and methods.
// Panics if fn is nil.
// Func 使用 http.HandlerFunc 构造 Blueprint 必填 handler。
// 这是普通 handler 函数和方法值的首选写法。
// fn 为 nil 时会 panic。
func Func(fn http.HandlerFunc) handlerSpec {
	return handlerSpec{handler: mustHandler(fn)}
}

func mustHandler(h http.Handler) http.Handler {
	if isNilHandler(h) {
		panic("routes: nil handler")
	}
	return h
}

func isNilHandler(h http.Handler) bool {
	if h == nil {
		return true
	}
	v := reflect.ValueOf(h)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

// IncludeOption modifies child routes during Include.
// IncludeOption 在 Include 时修改子路由。
type IncludeOption func(*includeConfig)

type includeConfig struct {
	tags       []string
	auth       AuthPolicy
	hasAuth    bool
	middleware []Middleware
}

// IncludeTags appends tags to included routes.
// IncludeTags 为 include 的路由追加 tags。
func IncludeTags(tags ...string) IncludeOption {
	return func(c *includeConfig) {
		c.tags = append(c.tags, tags...)
	}
}

// IncludeAuth sets the auth policy on included routes.
// IncludeAuth 为 include 的路由设置认证策略。
func IncludeAuth(policy AuthPolicy) IncludeOption {
	return func(c *includeConfig) {
		c.auth = policy
		c.hasAuth = true
	}
}

// IncludeMiddleware prepends middleware to included routes.
// IncludeMiddleware 为 include 的路由预置最外层中间件。
func IncludeMiddleware(mw ...Middleware) IncludeOption {
	return func(c *includeConfig) {
		c.middleware = append(c.middleware, mw...)
	}
}

// NewBlueprint returns an empty Blueprint.
// NewBlueprint 返回空 Blueprint。
func NewBlueprint(opts ...BlueprintOption) *Blueprint {
	var cfg blueprintConfig
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return &Blueprint{
		routes:     make([]Route, 0),
		tags:       append([]string(nil), cfg.tags...),
		auth:       cfg.auth,
		hasAuth:    cfg.hasAuth,
		middleware: append([]Middleware(nil), cfg.middleware...),
	}
}

// Add adds routes.
// The blueprint keeps its own copies.
// Add 添加路由。
// Blueprint 会保留自己的副本。
func (b *Blueprint) Add(routeList ...Route) {
	b = requireBlueprint(b)
	if len(routeList) == 0 {
		return
	}
	owned := cloneRoutes(routeList)
	for i := range owned {
		owned[i] = b.withDefaults(owned[i])
	}
	b.routes = append(b.routes, owned...)
}

func (b *Blueprint) withDefaults(route Route) Route {
	if len(b.tags) > 0 {
		tags := append([]string(nil), b.tags...)
		route.Tags = append(tags, route.Tags...)
	}
	if b.hasAuth && route.Auth == "" {
		route.Auth = b.auth
	}
	if len(b.middleware) > 0 {
		mw := append([]Middleware(nil), b.middleware...)
		route.Middleware = append(mw, route.Middleware...)
	}
	return route
}

// Handle adds a route.
// The handler is required as the fourth argument.
// Panics if spec is invalid.
// Handle 添加路由。
// 第四个参数必须提供 handler。
// spec 非法时会 panic。
func (b *Blueprint) Handle(method, path, summary string, spec handlerSpec, opts ...RouteOption) {
	b = requireBlueprint(b)
	if spec.handler == nil {
		panic("routes: missing handler")
	}
	route := Route{
		Method:  method,
		Path:    path,
		Summary: summary,
		Handler: spec.handler,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&route)
		}
	}
	b.Add(route)
}

// Get adds a GET route.
// Get 添加 GET 路由。
func (b *Blueprint) Get(path, summary string, spec handlerSpec, opts ...RouteOption) {
	b.Handle(http.MethodGet, path, summary, spec, opts...)
}

// Post adds a POST route.
// Post 添加 POST 路由。
func (b *Blueprint) Post(path, summary string, spec handlerSpec, opts ...RouteOption) {
	b.Handle(http.MethodPost, path, summary, spec, opts...)
}

// Put adds a PUT route.
// Put 添加 PUT 路由。
func (b *Blueprint) Put(path, summary string, spec handlerSpec, opts ...RouteOption) {
	b.Handle(http.MethodPut, path, summary, spec, opts...)
}

// Patch adds a PATCH route.
// Patch 添加 PATCH 路由。
func (b *Blueprint) Patch(path, summary string, spec handlerSpec, opts ...RouteOption) {
	b.Handle(http.MethodPatch, path, summary, spec, opts...)
}

// Delete adds a DELETE route.
// Delete 添加 DELETE 路由。
func (b *Blueprint) Delete(path, summary string, spec handlerSpec, opts ...RouteOption) {
	b.Handle(http.MethodDelete, path, summary, spec, opts...)
}

// Include adds child routes under prefix.
// Include 在 prefix 下添加子路由。
func (b *Blueprint) Include(prefix string, child *Blueprint, opts ...IncludeOption) {
	b = requireBlueprint(b)
	child = requireBlueprint(child)

	var cfg includeConfig
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	routeList := child.Routes()
	if len(cfg.tags) > 0 {
		routeList = WithTags(routeList, cfg.tags...)
	}
	if cfg.hasAuth {
		for i := range routeList {
			routeList[i].Auth = cfg.auth
		}
	}
	if len(cfg.middleware) > 0 {
		routeList = WithMiddleware(routeList, cfg.middleware...)
	}
	b.Add(ownedRoutes(prefix, routeList)...)
}

// Routes returns a copy of the routes.
// Routes 返回路由副本。
func (b *Blueprint) Routes() []Route {
	b = requireBlueprint(b)
	return cloneRoutes(b.routes)
}

// Mount registers the routes on router.
// Mount 在 router 上注册路由。
func (b *Blueprint) Mount(router chi.Router, opts ...MountOption) error {
	return b.MountAt(router, "", opts...)
}

// MountAt registers the routes under prefix.
// MountAt 在 prefix 下注册路由。
func (b *Blueprint) MountAt(router chi.Router, prefix string, opts ...MountOption) error {
	b = requireBlueprint(b)
	return Mount(router, prefix, b.routes, opts...)
}

// ExportJSON exports route metadata as JSON.
// ExportJSON 将路由元数据导出为 JSON。
func (b *Blueprint) ExportJSON() ([]byte, error) {
	b = requireBlueprint(b)
	return ExportJSON(b.routes)
}

// ExportMarkdown exports route metadata as Markdown.
// ExportMarkdown 将路由元数据导出为 Markdown。
func (b *Blueprint) ExportMarkdown() (string, error) {
	b = requireBlueprint(b)
	return ExportMarkdown(b.routes)
}

func requireBlueprint(b *Blueprint) *Blueprint {
	if b == nil {
		panic("routes: nil Blueprint")
	}
	return b
}
