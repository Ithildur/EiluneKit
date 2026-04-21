// Package routes provides declarative route definitions with automatic auth middleware.
// Package routes 提供声明式路由定义与自动认证中间件。
package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
)

// AuthPolicy is the route authentication requirement.
// AuthPolicy 表示路由认证要求。
type AuthPolicy string

const (
	AuthNone           AuthPolicy = "none"
	AuthBearerRequired AuthPolicy = "bearer_required"
	AuthBearerOptional AuthPolicy = "bearer_optional"
	AuthAPIKey         AuthPolicy = "api_key"
	AuthRefreshCookie  AuthPolicy = "refresh_cookie"
)

// Middleware is a chi-compatible middleware.
// Middleware 是兼容 chi 的中间件。
type Middleware = func(http.Handler) http.Handler

// Route defines an HTTP endpoint.
// Pass routes to Mount, optionally with WithAuth.
// Route 定义 HTTP 端点。
// 使用 Mount 挂载路由；需要认证时配合 WithAuth。
type Route struct {
	Method     string
	Path       string
	Summary    string
	Tags       []string
	Auth       AuthPolicy
	Handler    http.Handler
	Middleware []Middleware
}

// Clone returns a copy of r.
// Clone 返回 r 的副本。
func (r Route) Clone() Route {
	out := r
	if r.Tags != nil {
		out.Tags = append([]string(nil), r.Tags...)
	}
	if r.Middleware != nil {
		out.Middleware = append([]Middleware(nil), r.Middleware...)
	}
	return out
}

// AuthResolver maps AuthPolicy to middleware.
// Mount requires entries for every protected route.
// AuthResolver 将 AuthPolicy 映射到中间件。
// Mount 要求每个受保护路由都有对应映射。
type AuthResolver map[AuthPolicy]Middleware

type MountOption func(*mountConfig)

type mountConfig struct {
	authResolver AuthResolver
}

// WithAuth applies auth middleware from resolver during Mount.
// Every protected AuthPolicy must exist in resolver.
// WithAuth 在 Mount 时按 resolver 应用认证中间件。
// 每个受保护的 AuthPolicy 都必须在 resolver 中存在映射。
func WithAuth(resolver AuthResolver) MountOption {
	return func(c *mountConfig) {
		c.authResolver = resolver
	}
}

// Mount registers routes on r.
// Mount does not mutate routes.
// Missing auth middleware is a mount error.
// Mount 在 r 上注册路由。
// Mount 不会修改 routes。
// 缺少认证中间件会直接报错。
func Mount(r chi.Router, prefix string, routes []Route, opts ...MountOption) error {
	if r == nil {
		return fmt.Errorf("routes: nil chi.Router")
	}

	var cfg mountConfig
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	p := cleanPrefix(prefix)
	if p == "" {
		return mountRoutes(r, routes, cfg.authResolver)
	}

	var mountErr error
	r.Route(p, func(r chi.Router) {
		if err := mountRoutes(r, routes, cfg.authResolver); err != nil && mountErr == nil {
			mountErr = err
		}
	})
	return mountErr
}

func mountRoutes(r chi.Router, routes []Route, authResolver AuthResolver) error {
	seen := make(map[string]struct{}, len(routes))

	for i, raw := range routes {
		method, path, err := normalizeRoute(raw.Method, raw.Path)
		if err != nil {
			return fmt.Errorf("routes: route[%d]: %w", i, err)
		}
		if raw.Handler == nil {
			return fmt.Errorf("routes: route[%d] %s %s: nil handler", i, method, path)
		}

		key := method + " " + path
		if _, ok := seen[key]; ok {
			return fmt.Errorf("routes: route[%d] duplicate: %s", i, key)
		}
		seen[key] = struct{}{}

		handler := raw.Handler

		for j := len(raw.Middleware) - 1; j >= 0; j-- {
			if raw.Middleware[j] != nil {
				handler = raw.Middleware[j](handler)
			}
		}

		authPolicy := effectiveAuth(raw.Auth)
		if authPolicy != AuthNone {
			if authResolver == nil {
				return fmt.Errorf("routes: route[%d] %s %s: missing middleware for auth policy %q", i, method, path, authPolicy)
			}
			mw, ok := authResolver[authPolicy]
			if !ok || mw == nil {
				return fmt.Errorf("routes: route[%d] %s %s: missing middleware for auth policy %q", i, method, path, authPolicy)
			}
			handler = mw(handler)
		}

		r.Method(method, path, handler)
	}
	return nil
}

func normalizeRoute(methodRaw, pathRaw string) (string, string, error) {
	method := strings.ToUpper(strings.TrimSpace(methodRaw))
	if method == "" {
		return "", "", fmt.Errorf("empty method for path=%q", strings.TrimSpace(pathRaw))
	}

	path := strings.TrimSpace(pathRaw)
	if path == "" {
		return method, "/", nil
	}
	if strings.HasPrefix(path, "/") {
		return method, path, nil
	}
	return method, "/" + path, nil
}

type exportRoute struct {
	Method  string     `json:"method"`
	Path    string     `json:"path"`
	Summary string     `json:"summary"`
	Tags    []string   `json:"tags,omitempty"`
	Auth    AuthPolicy `json:"auth"`
}

// ExportJSON returns route metadata as JSON.
// ExportJSON 返回 JSON 路由元数据。
func ExportJSON(routes []Route) ([]byte, error) {
	exported := buildExportRoutes(routes)
	sortExportRoutes(exported)
	return json.Marshal(exported)
}

// ExportMarkdown returns a Markdown route table.
// ExportMarkdown 返回 Markdown 路由表。
func ExportMarkdown(routes []Route) (string, error) {
	lines := []string{
		"| Method | Path | Summary | Auth | Tags |",
		"|---|---|---|---|---|",
	}

	exported := buildExportRoutes(routes)
	sortExportRoutes(exported)

	for _, rt := range exported {
		tags := strings.Join(rt.Tags, ", ")
		line := fmt.Sprintf("| %s | %s | %s | %s | %s |",
			rt.Method,
			rt.Path,
			sanitizeMarkdownCell(rt.Summary),
			rt.Auth,
			sanitizeMarkdownCell(tags),
		)
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n"), nil
}

func sortExportRoutes(exported []exportRoute) {
	for i := range exported {
		if len(exported[i].Tags) > 1 {
			sort.Strings(exported[i].Tags)
		}
	}

	sort.SliceStable(exported, func(i, j int) bool {
		if exported[i].Path == exported[j].Path {
			return exported[i].Method < exported[j].Method
		}
		return exported[i].Path < exported[j].Path
	})
}

// WithPrefix returns routes with prefix applied.
// WithPrefix 返回添加 prefix 后的路由副本。
func WithPrefix(prefix string, routes []Route) []Route {
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

func joinPath(prefix, path string) string {
	if path == "" || path == "/" {
		return prefix
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return prefix + path
}

func cleanPrefix(prefix string) string {
	p := strings.TrimSpace(prefix)
	if p == "" || p == "/" {
		return ""
	}
	p = strings.TrimSuffix(p, "/")
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}

func sanitizeMarkdownCell(s string) string {
	if s == "" {
		return s
	}
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return strings.ReplaceAll(s, "|", "\\|")
}

func buildExportRoutes(routes []Route) []exportRoute {
	exported := make([]exportRoute, 0, len(routes))
	for _, raw := range routes {
		exported = append(exported, exportRoute{
			Method:  strings.ToUpper(strings.TrimSpace(raw.Method)),
			Path:    normalizePathForExport(raw.Path),
			Summary: raw.Summary,
			Tags:    append([]string(nil), raw.Tags...),
			Auth:    effectiveAuth(raw.Auth),
		})
	}
	return exported
}

func normalizePathForExport(pathRaw string) string {
	p := strings.TrimSpace(pathRaw)
	if p == "" {
		return "/"
	}
	if strings.HasPrefix(p, "/") {
		return p
	}
	return "/" + p
}

func effectiveAuth(auth AuthPolicy) AuthPolicy {
	if auth == "" {
		return AuthNone
	}
	return auth
}

func cloneRoutes(routes []Route) []Route {
	if len(routes) == 0 {
		return nil
	}

	out := make([]Route, len(routes))
	for i := range routes {
		out[i] = routes[i].Clone()
	}
	return out
}
