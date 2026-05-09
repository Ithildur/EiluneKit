// Package routes provides declarative route definitions and middleware composition.
// Package routes 提供声明式路由定义与中间件组合。
package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
)

// Middleware is a chi-compatible middleware.
// Middleware 是兼容 chi 的中间件。
type Middleware = func(http.Handler) http.Handler

// AuthRequirement is the exported authentication requirement for a route.
// It is metadata only; Mount never applies middleware from this field.
// AuthRequirement 表示导出的路由认证要求。
// 它只作为元数据；Mount 永远不会根据该字段应用中间件。
type AuthRequirement string

const (
	// AuthPublic means no authentication is required.
	// AuthPublic 表示不要求认证。
	AuthPublic AuthRequirement = "public"
	// AuthOptional means authentication may be supplied but is not required.
	// AuthOptional 表示可以提供认证但不强制要求。
	AuthOptional AuthRequirement = "optional"
	// AuthRequired means authentication is required.
	// AuthRequired 表示必须认证。
	AuthRequired AuthRequirement = "required"
)

// Route defines an HTTP endpoint.
// Pass routes to Mount.
// Route 定义 HTTP 端点。
// 使用 Mount 挂载路由。
type Route struct {
	Method     string
	Path       string
	Summary    string
	Tags       []string
	Auth       AuthRequirement
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

// Mount registers routes on r.
// Mount does not mutate routes.
// Mount 在 r 上注册路由。
// Mount 不会修改 routes。
func Mount(r chi.Router, prefix string, routes []Route) error {
	if r == nil {
		return fmt.Errorf("routes: nil chi.Router")
	}

	p := cleanPrefix(prefix)
	if p == "" {
		return mountRoutes(r, routes)
	}

	var mountErr error
	r.Route(p, func(r chi.Router) {
		if err := mountRoutes(r, routes); err != nil && mountErr == nil {
			mountErr = err
		}
	})
	return mountErr
}

func mountRoutes(r chi.Router, routes []Route) error {
	seen := make(map[string]struct{}, len(routes))

	for i, raw := range routes {
		method, path, err := normalizeRoute(raw.Method, raw.Path)
		if err != nil {
			return fmt.Errorf("routes: route[%d]: %w", i, err)
		}
		if isNilHandler(raw.Handler) {
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
	Method  string          `json:"method"`
	Path    string          `json:"path"`
	Summary string          `json:"summary"`
	Tags    []string        `json:"tags,omitempty"`
	Auth    AuthRequirement `json:"auth"`
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

func effectiveAuth(auth AuthRequirement) AuthRequirement {
	if auth == "" {
		return AuthPublic
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
