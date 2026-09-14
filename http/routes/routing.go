// Package routes provides declarative route definitions and middleware composition.
// Package routes 提供声明式路由定义与中间件组合。
package routes

import (
	"encoding/json/v2"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/Ithildur/EiluneKit/internal/routepath"
	"github.com/go-chi/chi/v5"
)

// Middleware is a chi-compatible middleware.
// Middleware 是兼容 chi 的中间件。
type Middleware = func(http.Handler) http.Handler

// AuthRequirement is the exported authentication requirement for a route.
// Mount guards AuthRequired routes at runtime.
// AuthRequirement 表示导出的路由认证要求。
// Mount 会在运行时保护 AuthRequired 路由。
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
	Method string
	// Path starts with a single slash, or is empty to select the mount directory itself.
	// A lone slash selects its trailing-slash endpoint.
	// Path 以单个斜线开头，或用空字符串选择挂载目录本身；单独的斜线选择其带尾斜线端点。
	Path        string
	Summary     string
	Tags        []string
	Auth        AuthRequirement
	Handler     http.Handler
	Middleware  []Middleware
	OperationID string
	Parameters  []Parameter
	RequestBody *RequestBody
	Responses   map[string]Response
	Security    []SecurityRequirement
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
	out.Parameters = cloneParameters(r.Parameters)
	if r.RequestBody != nil {
		out.RequestBody = new(r.RequestBody.clone())
	}
	out.Responses = cloneResponses(r.Responses)
	out.Security = cloneSecurity(r.Security)
	return out
}

// Mount registers routes on r.
// Prefixes are expanded before registration; Mount does not create subrouters or mutate routes.
// Prefix must be relative without a trailing slash; empty selects the current directory.
// Conflicts within the batch or with r.Routes() panic before registration; see [MountWithOptions].
// Mount 在 r 上注册路由。
// 前缀在注册前展开；Mount 不会创建子路由或修改 routes。
// prefix 必须是无尾斜线的相对目录；空字符串选择当前目录。
// 本批路由内部或与 r.Routes() 的冲突会在注册前 panic；参见 [MountWithOptions]。
func Mount(r chi.Router, prefix string, routes []Route) error {
	return MountWithOptions(r, prefix, routes, MountOptions{})
}

// MountOptions configures the runtime authentication guard.
// MountOptions 配置运行时认证保护。
type MountOptions struct {
	// Unauthorized handles requests without an authenticated marker on required routes.
	// Nil preserves the default JSON 401 response. It cannot grant access to the endpoint.
	// Unauthorized 处理必需认证路由中缺少认证标记的请求。
	// Nil 保留默认 JSON 401 响应；该 handler 不能放行到端点。
	Unauthorized http.Handler
}

// MountWithOptions registers routes with an application-supplied authentication failure response.
// Route middleware must still mark successful authentication with WithAuthenticated.
// Registration conflicts panic, as in [Mount]. Direct chi registrations bypass this check;
// chi's hidden Mount forwarding aliases are not included in r.Routes().
// MountWithOptions 注册路由并允许应用提供认证失败响应。
// 路由中间件仍须在认证成功后调用 WithAuthenticated。
// 注册冲突与 [Mount] 一样会 panic。直接调用 chi 注册会绕过检查；
// r.Routes() 不包含 chi 隐藏的 Mount 转发别名。
func MountWithOptions(r chi.Router, prefix string, routes []Route, opts MountOptions) error {
	if r == nil {
		return fmt.Errorf("routes: nil chi.Router")
	}

	if err := routepath.ValidatePrefix(prefix); err != nil {
		return fmt.Errorf("routes: %w", err)
	}
	probe := chi.NewRouter()
	index := newRouteIndex(probe, r.Routes())
	prepared := make([]Route, len(routes))

	for i, raw := range routes {
		if err := routepath.Validate(raw.Path); err != nil {
			return fmt.Errorf("routes: route[%d]: %w", i, err)
		}
		method, path, err := normalizeRoute(raw.Method, routepath.Join(prefix, raw.Path))
		if err != nil {
			return fmt.Errorf("routes: route[%d]: %w", i, err)
		}
		if isNilHandler(raw.Handler) {
			return fmt.Errorf("routes: route[%d] %s %s: nil handler", i, method, path)
		}

		handler := raw.Handler
		if params, ok := handler.(*paramHandler); ok {
			handler, err = params.bindPath(path)
			if err != nil {
				return fmt.Errorf("routes: route[%d] %s %s: %w", i, method, path, err)
			}
		}
		switch auth := effectiveAuth(raw.Auth); auth {
		case AuthPublic, AuthOptional:
		case AuthRequired:
			handler = requireAuthenticated(handler, opts.Unauthorized)
		default:
			return fmt.Errorf("routes: route[%d] %s %s: unsupported auth requirement %q", i, method, path, auth)
		}

		// Let chi validate its method and pattern syntax without touching the live router.
		// 让 chi 校验其方法和模式语法，避免修改实际路由器。
		probe.Method(method, path, handler)
		if err := index.add(method, path); err != nil {
			panic(fmt.Sprintf("routes: route[%d] %s %s: %v", i, method, path, err))
		}
		prepared[i] = raw
		prepared[i].Method, prepared[i].Path, prepared[i].Handler = method, path, handler
	}

	for _, rt := range prepared {
		handler := rt.Handler

		for _, middleware := range slices.Backward(rt.Middleware) {
			if middleware != nil {
				handler = middleware(handler)
			}
		}

		r.Method(rt.Method, rt.Path, handler)
	}
	return nil
}

func normalizeRoute(methodRaw, pathRaw string) (string, string, error) {
	method := strings.ToUpper(strings.TrimSpace(methodRaw))
	if method == "" {
		return "", "", fmt.Errorf("empty method for path=%q", strings.TrimSpace(pathRaw))
	}

	path, err := routepath.Pattern(pathRaw)
	return method, path, err
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
	exported, err := buildExportRoutes(routes)
	if err != nil {
		return nil, err
	}
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

	exported, err := buildExportRoutes(routes)
	if err != nil {
		return "", err
	}
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
// Empty paths add no suffix; slash paths preserve their trailing slash.
// Dynamic prefix parameters default to required string path parameters.
// Existing path parameter metadata takes precedence.
// Non-empty paths start with a slash. Invalid path or prefix syntax panics.
// WithPrefix 返回添加 prefix 后的路由副本。
// 空 path 不添加后缀；斜线路径保留尾斜线。
// 动态前缀参数默认成为必填的 string path 参数。
// 已有的 path 参数元数据优先。
// 非空路径以斜线开头；路径或前缀语法无效时 panic。
func WithPrefix(prefix string, routes []Route) []Route {
	if err := routepath.ValidatePrefix(prefix); err != nil {
		panic("routes: " + err.Error())
	}
	if len(routes) == 0 {
		return nil
	}

	out := cloneRoutes(routes)
	for i := range out {
		if err := routepath.Validate(out[i].Path); err != nil {
			panic("routes: " + err.Error())
		}
		out[i].Path = routepath.Join(prefix, out[i].Path)
		out[i].Parameters = withPrefixParameters(prefix, out[i].Parameters)
	}
	return out
}

func withPrefixParameters(prefix string, params []Parameter) []Parameter {
	names := pathParamNames(prefix)
	if len(names) == 0 {
		return params
	}

	declared := make(map[string]struct{}, len(params))
	for _, param := range params {
		if param.In == ParameterPath {
			declared[param.Name] = struct{}{}
		}
	}

	inferred := make([]Parameter, 0, len(names))
	for _, name := range names {
		if name == "*" {
			continue
		}
		if _, exists := declared[name]; exists {
			continue
		}
		inferred = append(inferred, Parameter{
			Name:     name,
			In:       ParameterPath,
			Required: true,
			Schema:   SchemaOf[string](""),
		})
		declared[name] = struct{}{}
	}
	return append(inferred, params...)
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

func buildExportRoutes(routes []Route) ([]exportRoute, error) {
	exported := make([]exportRoute, 0, len(routes))
	for i, raw := range routes {
		path, err := routepath.Pattern(raw.Path)
		if err != nil {
			return nil, fmt.Errorf("routes: route[%d]: %w", i, err)
		}
		exported = append(exported, exportRoute{
			Method:  strings.ToUpper(strings.TrimSpace(raw.Method)),
			Path:    path,
			Summary: raw.Summary,
			Tags:    append([]string(nil), raw.Tags...),
			Auth:    effectiveAuth(raw.Auth),
		})
	}
	return exported, nil
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
