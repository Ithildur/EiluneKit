package routes

import (
	"fmt"
	"strings"

	"github.com/go-chi/chi/v5"
)

// routeIndex exists only during a mount; the chi router owns the registered routes.
// routeIndex 仅在挂载期间存在；已注册路由由 chi router 持有。
type routeIndex struct {
	routes []chi.Route
	roots  map[string]*routeNode
}

type routeNode struct {
	static   map[byte]*routeNode
	params   map[paramEdge]*routeNode
	names    map[string]string
	path     string
	endpoint string
	catchAll string
}

// Regex and delimiter branches retain chi's matching rules.
// 正则和分隔符分支保留 chi 的匹配规则。
type paramEdge struct {
	expression string
	tail       byte
	regexp     bool
}

func newRouteIndex(probe *chi.Mux, routes []chi.Route) routeIndex {
	// chi hides concrete Mount forwarding methods; replay fallbacks before explicit overrides.
	// chi 隐藏了 Mount 转发器的具体方法；先恢复转发记录，再应用显式覆盖。
	for _, rt := range routes {
		if rt.SubRoutes != nil && rt.Handlers["*"] != nil {
			probe.Handle(rt.Pattern, rt.Handlers["*"])
		}
	}
	for _, rt := range routes {
		for method, handler := range rt.Handlers {
			if method != "*" {
				probe.Method(method, rt.Pattern, handler)
			}
		}
	}
	return routeIndex{routes: probe.Routes(), roots: make(map[string]*routeNode)}
}

func (idx *routeIndex) add(method, path string) error {
	root := idx.roots[method]
	if root == nil {
		root = &routeNode{}
		for _, rt := range idx.routes {
			// chi expands Handle into concrete methods; "*" can remain after an override.
			// chi 将 Handle 展开为具体方法；覆盖后仍可能保留 "*"。
			if rt.Handlers[method] == nil {
				continue
			}
			root.insert(rt.Pattern, false)
		}
		idx.roots[method] = root
	}
	return root.insert(path, true)
}

// Existing chi routes may already overlap; only reject conflicts introduced by this mount.
// 已有 chi 路由可能相互重叠；只拒绝本次挂载引入的冲突。
func (n *routeNode) insert(path string, check bool) error {
	for i := 0; ; {
		if check && n.catchAll != "" {
			return fmt.Errorf("conflicts with catch-all %q", n.catchAll)
		}
		if i == len(path) {
			if check && n.endpoint != "" {
				return fmt.Errorf("duplicate route conflicts with %q", n.endpoint)
			}
			n.endpoint = path
			if n.path == "" {
				n.path = path
			}
			return nil
		}
		if path[i] == '*' {
			if check && n.path != "" {
				return fmt.Errorf("catch-all conflicts with %q", n.path)
			}
			n.catchAll = path
			n.path = path
			return nil
		}
		if n.path == "" {
			n.path = path
		}
		if path[i] != '{' {
			if n.static == nil {
				n.static = make(map[byte]*routeNode)
			}
			child := n.static[path[i]]
			if child == nil {
				child = &routeNode{}
				n.static[path[i]] = child
			}
			n = child
			i++
			continue
		}

		end := pathParamEnd(path, i)
		name, expression, regex := strings.Cut(path[i+1:end], ":")
		if expression != "" {
			if !strings.HasPrefix(expression, "^") {
				expression = "^" + expression
			}
			if !strings.HasSuffix(expression, "$") {
				expression += "$"
			}
		}
		tail := byte('/')
		if end+1 < len(path) {
			tail = path[end+1]
		}
		key := paramEdge{expression: expression, tail: tail, regexp: regex}
		if n.params == nil {
			n.params = make(map[paramEdge]*routeNode)
		}
		child := n.params[key]
		if child == nil {
			child = &routeNode{names: make(map[string]string)}
			n.params[key] = child
		}
		if check {
			for existing, source := range child.names {
				if existing != name {
					return fmt.Errorf("parameter %q conflicts with parameter %q in %q", name, existing, source)
				}
			}
		}
		child.names[name] = path
		n = child
		i = end + 1
	}
}
