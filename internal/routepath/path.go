// Package routepath defines endpoint paths and mount directories.
// Package routepath 定义端点路径和挂载目录。
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

// ValidatePrefix checks a mount directory. Empty means the current directory.
// ValidatePrefix 检查挂载目录；空字符串表示当前目录。
func ValidatePrefix(prefix string) error {
	if strings.TrimSpace(prefix) != prefix {
		return fmt.Errorf("route prefix %q must not contain surrounding whitespace", prefix)
	}
	if strings.HasPrefix(prefix, "/") {
		return fmt.Errorf("route prefix %q must not start with a slash; use an empty prefix for the current directory", prefix)
	}
	if strings.HasSuffix(prefix, "/") {
		return fmt.Errorf("route prefix %q must not end with a slash; use an empty prefix for the current directory", prefix)
	}
	return nil
}

// Join combines a validated mount directory and endpoint path, preserving trailing slashes.
// Join 组合已校验的挂载目录和端点路径，保留尾斜线。
func Join(prefix, path string) string {
	if prefix == "" {
		return path
	}
	return "/" + prefix + path
}

// Pattern validates an endpoint path and resolves an empty final path to the HTTP root.
// Pattern 校验端点路径，将空的最终路径解析为 HTTP 根路径。
func Pattern(path string) (string, error) {
	if err := Validate(path); err != nil {
		return "", err
	}
	if path == "" {
		return "/", nil
	}
	return path, nil
}
