package routes

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
)

// HandlerFunc is the set of handler signatures accepted by Blueprint methods.
// Up to 15 extra strings receive dynamic path values in final route order.
// HandlerFunc 是 Blueprint 方法接受的 handler 签名集合。
// 最多 15 个额外 string 参数按最终路由顺序接收动态 path 值。
type HandlerFunc interface {
	~func(http.ResponseWriter, *http.Request) |
		~func(http.ResponseWriter, *http.Request, string) |
		~func(http.ResponseWriter, *http.Request, string, string) |
		~func(http.ResponseWriter, *http.Request, string, string, string) |
		~func(http.ResponseWriter, *http.Request, string, string, string, string) |
		~func(http.ResponseWriter, *http.Request, string, string, string, string, string) |
		~func(http.ResponseWriter, *http.Request, string, string, string, string, string, string) |
		~func(http.ResponseWriter, *http.Request, string, string, string, string, string, string, string) |
		~func(http.ResponseWriter, *http.Request, string, string, string, string, string, string, string, string) |
		~func(http.ResponseWriter, *http.Request, string, string, string, string, string, string, string, string, string) |
		~func(http.ResponseWriter, *http.Request, string, string, string, string, string, string, string, string, string, string) |
		~func(http.ResponseWriter, *http.Request, string, string, string, string, string, string, string, string, string, string, string) |
		~func(http.ResponseWriter, *http.Request, string, string, string, string, string, string, string, string, string, string, string, string) |
		~func(http.ResponseWriter, *http.Request, string, string, string, string, string, string, string, string, string, string, string, string, string) |
		~func(http.ResponseWriter, *http.Request, string, string, string, string, string, string, string, string, string, string, string, string, string, string) |
		~func(http.ResponseWriter, *http.Request, string, string, string, string, string, string, string, string, string, string, string, string, string, string, string)
}

// paramHandler is a declaration; binding creates an independent request handler.
// paramHandler 是声明；绑定会创建独立的请求 handler。
type paramHandler struct {
	count int
	bind  func([]string) http.HandlerFunc
}

func newFuncHandler[H HandlerFunc](fn H) http.Handler {
	v := reflect.ValueOf(fn)
	if v.IsNil() {
		panic("routes: nil handler function")
	}
	switch v.Type().NumIn() - 2 {
	case 0:
		f := convertFunc[func(http.ResponseWriter, *http.Request)](v)
		return http.HandlerFunc(f)
	case 1:
		f := convertFunc[func(http.ResponseWriter, *http.Request, string)](v)
		return &paramHandler{count: 1, bind: func(names []string) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				f(w, r, r.PathValue(names[0]))
			}
		}}
	case 2:
		f := convertFunc[func(http.ResponseWriter, *http.Request, string, string)](v)
		return &paramHandler{count: 2, bind: func(names []string) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				f(w, r, r.PathValue(names[0]), r.PathValue(names[1]))
			}
		}}
	case 3:
		f := convertFunc[func(http.ResponseWriter, *http.Request, string, string, string)](v)
		return &paramHandler{count: 3, bind: func(names []string) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				f(w, r, r.PathValue(names[0]), r.PathValue(names[1]), r.PathValue(names[2]))
			}
		}}
	case 4:
		f := convertFunc[func(http.ResponseWriter, *http.Request, string, string, string, string)](v)
		return &paramHandler{count: 4, bind: func(names []string) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				f(w, r, r.PathValue(names[0]), r.PathValue(names[1]), r.PathValue(names[2]), r.PathValue(names[3]))
			}
		}}
	case 5:
		f := convertFunc[func(http.ResponseWriter, *http.Request, string, string, string, string, string)](v)
		return &paramHandler{count: 5, bind: func(names []string) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				f(w, r, r.PathValue(names[0]), r.PathValue(names[1]), r.PathValue(names[2]), r.PathValue(names[3]), r.PathValue(names[4]))
			}
		}}
	case 6:
		f := convertFunc[func(http.ResponseWriter, *http.Request, string, string, string, string, string, string)](v)
		return &paramHandler{count: 6, bind: func(names []string) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				f(w, r, r.PathValue(names[0]), r.PathValue(names[1]), r.PathValue(names[2]), r.PathValue(names[3]), r.PathValue(names[4]), r.PathValue(names[5]))
			}
		}}
	case 7:
		f := convertFunc[func(http.ResponseWriter, *http.Request, string, string, string, string, string, string, string)](v)
		return &paramHandler{count: 7, bind: func(names []string) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				f(w, r, r.PathValue(names[0]), r.PathValue(names[1]), r.PathValue(names[2]), r.PathValue(names[3]), r.PathValue(names[4]), r.PathValue(names[5]), r.PathValue(names[6]))
			}
		}}
	case 8:
		f := convertFunc[func(http.ResponseWriter, *http.Request, string, string, string, string, string, string, string, string)](v)
		return &paramHandler{count: 8, bind: func(names []string) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				f(w, r, r.PathValue(names[0]), r.PathValue(names[1]), r.PathValue(names[2]), r.PathValue(names[3]), r.PathValue(names[4]), r.PathValue(names[5]), r.PathValue(names[6]), r.PathValue(names[7]))
			}
		}}
	case 9:
		f := convertFunc[func(http.ResponseWriter, *http.Request, string, string, string, string, string, string, string, string, string)](v)
		return &paramHandler{count: 9, bind: func(names []string) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				f(w, r, r.PathValue(names[0]), r.PathValue(names[1]), r.PathValue(names[2]), r.PathValue(names[3]), r.PathValue(names[4]), r.PathValue(names[5]), r.PathValue(names[6]), r.PathValue(names[7]), r.PathValue(names[8]))
			}
		}}
	case 10:
		f := convertFunc[func(http.ResponseWriter, *http.Request, string, string, string, string, string, string, string, string, string, string)](v)
		return &paramHandler{count: 10, bind: func(names []string) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				f(w, r, r.PathValue(names[0]), r.PathValue(names[1]), r.PathValue(names[2]), r.PathValue(names[3]), r.PathValue(names[4]), r.PathValue(names[5]), r.PathValue(names[6]), r.PathValue(names[7]), r.PathValue(names[8]), r.PathValue(names[9]))
			}
		}}
	case 11:
		f := convertFunc[func(http.ResponseWriter, *http.Request, string, string, string, string, string, string, string, string, string, string, string)](v)
		return &paramHandler{count: 11, bind: func(names []string) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				f(w, r, r.PathValue(names[0]), r.PathValue(names[1]), r.PathValue(names[2]), r.PathValue(names[3]), r.PathValue(names[4]), r.PathValue(names[5]), r.PathValue(names[6]), r.PathValue(names[7]), r.PathValue(names[8]), r.PathValue(names[9]), r.PathValue(names[10]))
			}
		}}
	case 12:
		f := convertFunc[func(http.ResponseWriter, *http.Request, string, string, string, string, string, string, string, string, string, string, string, string)](v)
		return &paramHandler{count: 12, bind: func(names []string) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				f(w, r, r.PathValue(names[0]), r.PathValue(names[1]), r.PathValue(names[2]), r.PathValue(names[3]), r.PathValue(names[4]), r.PathValue(names[5]), r.PathValue(names[6]), r.PathValue(names[7]), r.PathValue(names[8]), r.PathValue(names[9]), r.PathValue(names[10]), r.PathValue(names[11]))
			}
		}}
	case 13:
		f := convertFunc[func(http.ResponseWriter, *http.Request, string, string, string, string, string, string, string, string, string, string, string, string, string)](v)
		return &paramHandler{count: 13, bind: func(names []string) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				f(w, r, r.PathValue(names[0]), r.PathValue(names[1]), r.PathValue(names[2]), r.PathValue(names[3]), r.PathValue(names[4]), r.PathValue(names[5]), r.PathValue(names[6]), r.PathValue(names[7]), r.PathValue(names[8]), r.PathValue(names[9]), r.PathValue(names[10]), r.PathValue(names[11]), r.PathValue(names[12]))
			}
		}}
	case 14:
		f := convertFunc[func(http.ResponseWriter, *http.Request, string, string, string, string, string, string, string, string, string, string, string, string, string, string)](v)
		return &paramHandler{count: 14, bind: func(names []string) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				f(w, r, r.PathValue(names[0]), r.PathValue(names[1]), r.PathValue(names[2]), r.PathValue(names[3]), r.PathValue(names[4]), r.PathValue(names[5]), r.PathValue(names[6]), r.PathValue(names[7]), r.PathValue(names[8]), r.PathValue(names[9]), r.PathValue(names[10]), r.PathValue(names[11]), r.PathValue(names[12]), r.PathValue(names[13]))
			}
		}}
	case 15:
		f := convertFunc[func(http.ResponseWriter, *http.Request, string, string, string, string, string, string, string, string, string, string, string, string, string, string, string)](v)
		return &paramHandler{count: 15, bind: func(names []string) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				f(w, r, r.PathValue(names[0]), r.PathValue(names[1]), r.PathValue(names[2]), r.PathValue(names[3]), r.PathValue(names[4]), r.PathValue(names[5]), r.PathValue(names[6]), r.PathValue(names[7]), r.PathValue(names[8]), r.PathValue(names[9]), r.PathValue(names[10]), r.PathValue(names[11]), r.PathValue(names[12]), r.PathValue(names[13]), r.PathValue(names[14]))
			}
		}}
	default:
		panic("routes: unsupported handler signature")
	}
}

// Normalize named function types once, before serving requests.
// 在处理请求前一次性归一化命名函数类型。
func convertFunc[F HandlerFunc](v reflect.Value) F {
	return v.Convert(reflect.TypeFor[F]()).Interface().(F)
}

func (*paramHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "route path params are not bound", http.StatusInternalServerError)
}

func (h *paramHandler) bindPath(path string) (http.Handler, error) {
	names := pathParamNames(path)
	if len(names) != h.count {
		return nil, fmt.Errorf("handler expects %d path params, route has %d", h.count, len(names))
	}
	if dup := duplicatePathParam(names); dup != "" {
		return nil, fmt.Errorf("duplicate path param %q", dup)
	}
	return h.bind(names), nil
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

func pathParamNames(path string) []string {
	names := make([]string, 0)
	for i := 0; i < len(path); i++ {
		switch path[i] {
		case '{':
			end := pathParamEnd(path, i)
			if end < 0 {
				return names
			}
			name, _, _ := strings.Cut(path[i+1:end], ":")
			if name != "" {
				names = append(names, name)
			}
			i = end
		case '*':
			if i+1 == len(path) {
				names = append(names, "*")
			}
		}
	}
	return names
}

func pathParamEnd(path string, start int) int {
	depth := 0
	for i := start; i < len(path); i++ {
		switch path[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func duplicatePathParam(names []string) string {
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if _, ok := seen[name]; ok {
			return name
		}
		seen[name] = struct{}{}
	}
	return ""
}
