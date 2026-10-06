// Package routepath defines endpoint paths and route prefixes.
// Package routepath 定义端点路径和路由前缀。
package routepath

import (
	"fmt"
	"strings"
)

// Validate checks a route path. A lone slash denotes a trailing-slash endpoint.
// Validate 检查路由路径；单独的斜线表示带尾斜线的端点。
func Validate(path string) error {
	if strings.TrimSpace(path) != path {
		return fmt.Errorf("route path %q must not contain surrounding whitespace", path)
	}
	if path != "" && (!strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//")) {
		return fmt.Errorf("route path %q must start with a single slash, or be empty", path)
	}
	return nil
}

// ValidatePrefix checks a route prefix. Empty means no prefix.
// ValidatePrefix 检查路由前缀；空字符串表示不添加前缀。
func ValidatePrefix(prefix string) error {
	if strings.TrimSpace(prefix) != prefix {
		return fmt.Errorf("route prefix %q must not contain surrounding whitespace", prefix)
	}
	if prefix != "" && (!strings.HasPrefix(prefix, "/") || strings.HasPrefix(prefix, "//")) {
		return fmt.Errorf("route prefix %q must start with a single slash, or be empty", prefix)
	}
	if strings.HasSuffix(prefix, "/") {
		return fmt.Errorf("route prefix %q must not end with a slash; use an empty prefix for the root", prefix)
	}
	return nil
}

// Pattern validates a non-empty final path.
// Pattern 校验非空的最终路径。
func Pattern(path string) (string, error) {
	if err := Validate(path); err != nil {
		return "", err
	}
	if path == "" {
		return "", fmt.Errorf("route path must not be empty; use \"/\" for the root")
	}
	return path, nil
}

// ParamEnd returns the closing brace index for the parameter at start, or -1 if unclosed.
// Nested braces in regular expressions are included.
// ParamEnd 返回 start 处参数的闭合括号索引；未闭合时返回 -1。
// 正则表达式中的嵌套括号也会计入。
func ParamEnd(path string, start int) int {
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
