package routes

import (
	"net/http"
	"slices"

	"github.com/Ithildur/EiluneKit/internal/routepath"
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
	auth       AuthRequirement
	hasAuth    bool
	middleware []Middleware
}

// BlueprintOption configures defaults for routes added to a Blueprint.
// BlueprintOption 配置 Blueprint 中新增路由的默认值。
type BlueprintOption func(*blueprintConfig)

type blueprintConfig struct {
	tags       []string
	auth       AuthRequirement
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

// DefaultAuth sets the default auth requirement.
// AuthRequired routes require authenticated context at runtime.
// DefaultAuth 设置默认认证要求。
// AuthRequired 路由运行时要求已认证 context。
func DefaultAuth(auth AuthRequirement) BlueprintOption {
	return func(c *blueprintConfig) {
		c.auth = auth
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

// Tags appends Route.Tags.
// Tags 追加 Route.Tags。
func Tags(tags ...string) RouteOption {
	return func(r *Route) {
		r.Tags = append(r.Tags, tags...)
	}
}

// Auth sets Route.Auth.
// AuthRequired routes require authenticated context at runtime.
// Auth 设置 Route.Auth。
// AuthRequired 路由运行时要求已认证 context。
func Auth(auth AuthRequirement) RouteOption {
	return func(r *Route) {
		r.Auth = auth
	}
}

// Use appends route middleware.
// Use 追加路由中间件。
func Use(mw ...Middleware) RouteOption {
	return func(r *Route) {
		r.Middleware = append(r.Middleware, mw...)
	}
}

// IncludeOption modifies child routes during Include.
// IncludeOption 在 Include 时修改子路由。
type IncludeOption func(*includeConfig)

type includeConfig struct {
	tags       []string
	auth       AuthRequirement
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

// IncludeAuth sets the auth requirement on included routes.
// AuthRequired routes require authenticated context at runtime.
// IncludeAuth 为 include 的路由设置认证要求。
// AuthRequired 路由运行时要求已认证 context。
func IncludeAuth(auth AuthRequirement) IncludeOption {
	return func(c *includeConfig) {
		c.auth = auth
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
// Panics if a non-empty route path does not start with a single slash or has surrounding whitespace.
// Add 添加路由。
// Blueprint 会保留自己的副本。
// 非空路径未以单个斜线开头或含首尾空白时 panic。
func (b *Blueprint) Add(routeList ...Route) {
	b = requireBlueprint(b)
	if len(routeList) == 0 {
		return
	}
	owned := cloneRoutes(routeList)
	for i := range owned {
		if err := routepath.Validate(owned[i].Path); err != nil {
			panic("routes: " + err.Error())
		}
		owned[i] = b.withDefaults(owned[i])
	}
	b.routes = append(b.routes, owned...)
}

func (b *Blueprint) withDefaults(route Route) Route {
	if len(b.tags) > 0 {
		route.Tags = slices.Concat(b.tags, route.Tags)
	}
	if b.hasAuth && route.Auth == "" {
		route.Auth = b.auth
	}
	if len(b.middleware) > 0 {
		route.Middleware = slices.Concat(b.middleware, route.Middleware)
	}
	return route
}

// Handle adds a route.
// Dynamic path values are passed to extra string arguments in route path order.
// Mount and include prefixes are part of that order. Up to 15 values are supported.
// Panics if fn is nil or path uses invalid endpoint syntax.
// Handle 添加路由。
// 动态 path 值会按路由 path 顺序传给额外的 string 参数。
// Mount 和 include 前缀也属于该顺序，最多支持 15 个值。
// fn 为 nil 或 path 不符合端点路径语法时 panic。
func (b *Blueprint) Handle[H HandlerFunc](method, path, summary string, fn H, opts ...RouteOption) {
	b = requireBlueprint(b)
	route := Route{
		Method:  method,
		Path:    path,
		Summary: summary,
		Handler: newFuncHandler(fn),
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
func (b *Blueprint) Get[H HandlerFunc](path, summary string, fn H, opts ...RouteOption) {
	b.Handle(http.MethodGet, path, summary, fn, opts...)
}

// Post adds a POST route.
// Post 添加 POST 路由。
func (b *Blueprint) Post[H HandlerFunc](path, summary string, fn H, opts ...RouteOption) {
	b.Handle(http.MethodPost, path, summary, fn, opts...)
}

// Put adds a PUT route.
// Put 添加 PUT 路由。
func (b *Blueprint) Put[H HandlerFunc](path, summary string, fn H, opts ...RouteOption) {
	b.Handle(http.MethodPut, path, summary, fn, opts...)
}

// Patch adds a PATCH route.
// Patch 添加 PATCH 路由。
func (b *Blueprint) Patch[H HandlerFunc](path, summary string, fn H, opts ...RouteOption) {
	b.Handle(http.MethodPatch, path, summary, fn, opts...)
}

// Delete adds a DELETE route.
// Delete 添加 DELETE 路由。
func (b *Blueprint) Delete[H HandlerFunc](path, summary string, fn H, opts ...RouteOption) {
	b.Handle(http.MethodDelete, path, summary, fn, opts...)
}

// Include adds child routes under prefix.
// Prefix follows [WithPrefix]; invalid prefixes and empty combined paths panic.
// Include 在 prefix 下添加子路由。
// prefix 遵循 [WithPrefix] 的规则；无效前缀或组合后路径为空时 panic。
func (b *Blueprint) Include(prefix string, child *Blueprint, opts ...IncludeOption) {
	b = requireBlueprint(b)
	child = requireBlueprint(child)

	var cfg includeConfig
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	routeList := WithPrefix(prefix, child.routes)
	for i := range routeList {
		route := &routeList[i]
		route.Tags = append(route.Tags, cfg.tags...)
		if cfg.hasAuth {
			route.Auth = cfg.auth
		}
		if len(cfg.middleware) > 0 {
			route.Middleware = slices.Concat(cfg.middleware, route.Middleware)
		}
		*route = b.withDefaults(*route)
	}
	b.routes = append(b.routes, routeList...)
}

// Routes returns a copy of the routes.
// Routes 返回路由副本。
func (b *Blueprint) Routes() []Route {
	b = requireBlueprint(b)
	return cloneRoutes(b.routes)
}

// RoutesAt returns route copies with prefix applied according to [WithPrefix].
// Use the returned routes for both mounting and API contract generation.
// Dynamic prefix parameters default to required string path parameters.
// RoutesAt 按 [WithPrefix] 的规则返回已添加 prefix 的路由副本。
// 将返回的路由同时用于挂载和 API 契约生成。
// 动态前缀参数默认成为必填的 string path 参数。
func (b *Blueprint) RoutesAt(prefix string) []Route {
	b = requireBlueprint(b)
	return WithPrefix(prefix, b.routes)
}

// Mount registers the routes on router.
// Registration conflicts panic; see [MountWithOptions].
// Mount 在 router 上注册路由。
// 注册冲突会 panic；参见 [MountWithOptions]。
func (b *Blueprint) Mount(router chi.Router) error {
	return b.MountAt(router, "")
}

// MountAt registers routes under prefix, following the path rules of [WithPrefix].
// Registration conflicts panic; see [MountWithOptions].
// MountAt 在 prefix 下注册路由，路径规则遵循 [WithPrefix]。
// 注册冲突会 panic；参见 [MountWithOptions]。
func (b *Blueprint) MountAt(router chi.Router, prefix string) error {
	b = requireBlueprint(b)
	return Mount(router, prefix, b.routes)
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
