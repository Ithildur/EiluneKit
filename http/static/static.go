// Package static provides static file and SPA helpers.
// Package static 提供静态文件与 SPA 辅助函数。
package static

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/Ithildur/EiluneKit/appdir"

	"github.com/go-chi/chi/v5"
)

var ErrInvalidProjectPath = errors.New("static: invalid project path")

// Options configures static asset discovery.
// Options 配置静态资源发现。
type Options struct {
	AppDir      appdir.Options
	Development bool
}

// SPAHandler returns a handler with SPA history fallback.
// relPath must be project-relative, for example "dist" or "web/dist".
// Call handler, err := SPAHandler(relPath, opts).
// SPAHandler 返回带 SPA history 回退的 handler。
// relPath 必须是项目相对路径，例如 "dist" 或 "web/dist"。
// 调用 handler, err := SPAHandler(relPath, opts)。
// Example / 示例:
//
//	handler, _ := static.SPAHandler("dist", static.Options{})
//	r.Handle("/*", handler)
func SPAHandler(relPath string, opts Options) (http.Handler, error) {
	dir, err := ResolveSPADir(relPath, opts)
	if err != nil {
		return nil, err
	}

	fsys := os.DirFS(dir)
	fileServer := http.FileServer(http.FS(fsys))
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if requestPath == "" {
			requestPath = "."
		}

		if _, err := fs.Stat(fsys, requestPath); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}

		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
	})

	return handler, nil
}

// MountSPA mounts a SPA handler at prefix.
// Call MountSPA(router, prefix, relPath, opts).
// MountSPA 在 prefix 挂载 SPA handler。
// 调用 MountSPA(router, prefix, relPath, opts)。
func MountSPA(router chi.Router, prefix string, relPath string, opts Options) (http.Handler, error) {
	handler, err := SPAHandler(relPath, opts)
	if err != nil {
		return nil, err
	}

	if prefix == "" || prefix == "/" {
		router.Handle("/*", handler)
		return handler, nil
	}
	router.Handle(prefix+"/*", http.StripPrefix(prefix, handler))
	return handler, nil
}

// Mount mounts a file server at prefix.
// Call Mount(router, prefix, relPath, opts).
// Mount 在 prefix 挂载文件服务器。
// 调用 Mount(router, prefix, relPath, opts)。
func Mount(router chi.Router, prefix string, relPath string, opts Options) error {
	dir, err := ResolveDir(relPath, opts)
	if err != nil {
		return err
	}

	fsys := os.DirFS(dir)
	handler := http.FileServer(http.FS(fsys))
	if prefix == "" || prefix == "/" {
		router.Handle("/*", handler)
		return nil
	}
	router.Handle(prefix+"/*", http.StripPrefix(prefix, handler))
	return nil
}

// ResolveDir returns the directory for relPath.
// relPath must be project-relative.
// ResolveDir 返回 relPath 对应目录。
// relPath 必须是项目相对路径。
func ResolveDir(relPath string, opts Options) (string, error) {
	return resolveDir(relPath, false, opts)
}

// ResolveSPADir returns the SPA directory for relPath.
// index.html must exist.
// ResolveSPADir 返回 relPath 对应的 SPA 目录。
// 必须存在 index.html。
func ResolveSPADir(relPath string, opts Options) (string, error) {
	return resolveDir(relPath, true, opts)
}

func resolveDir(relPath string, requireIndex bool, opts Options) (string, error) {
	normalized, relFSPath, err := cleanProjectPath(relPath)
	if err != nil {
		return "", err
	}

	appDirOpts := opts.AppDir
	if len(appDirOpts.Markers) == 0 {
		appDirOpts.Markers = []string{normalized, path.Join("web", normalized)}
		appDirOpts.RequireDirMarkers = true
	}
	if appDirOpts.Sources == 0 {
		appDirOpts.Sources = appdir.SourceEnvVar | appdir.SourceExecutable
		if opts.Development {
			appDirOpts.Sources |= appdir.SourceWorkingDir
		}
	}

	var roots []string
	home, err := appdir.DiscoverHome(appDirOpts)
	switch {
	case err == nil && home != "":
		roots = append(roots, home)
	case err != nil && errors.Is(err, appdir.ErrEnvInvalid):
		return "", fmt.Errorf("static: discover app home: %w", err)
	case err != nil && !errors.Is(err, appdir.ErrNoCandidates) && !errors.Is(err, appdir.ErrNotFound):
		return "", fmt.Errorf("static: discover app home: %w", err)
	}

	req := normalized
	if requireIndex {
		req = path.Join(normalized, "index.html")
	}

	for _, root := range roots {
		candidates := []string{root}
		if filepath.Base(root) != "web" {
			candidates = append(candidates, filepath.Join(root, "web"))
		}
		for _, base := range candidates {
			full := filepath.Join(base, relFSPath)
			if requireIndex {
				if fileExists(filepath.Join(full, "index.html")) {
					return full, nil
				}
				continue
			}
			if dirExists(full) {
				return full, nil
			}
		}
	}

	return "", fmt.Errorf("static: cannot find frontend assets in project path %q (required: %s)", normalized, req)
}

func cleanProjectPath(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("%w: path is empty", ErrInvalidProjectPath)
	}
	if filepath.IsAbs(raw) || filepath.VolumeName(raw) != "" {
		return "", "", fmt.Errorf("%w: %q must be relative to the project root", ErrInvalidProjectPath, raw)
	}

	normalized := strings.ReplaceAll(raw, "\\", "/")
	if strings.HasPrefix(normalized, "/") {
		return "", "", fmt.Errorf("%w: %q must be relative to the project root", ErrInvalidProjectPath, raw)
	}
	if path.Clean(normalized) != normalized {
		return "", "", fmt.Errorf("%w: %q must be a clean project-relative path without '.' or '..'", ErrInvalidProjectPath, raw)
	}

	for _, part := range strings.Split(normalized, "/") {
		if part == "" || part == "." || part == ".." {
			return "", "", fmt.Errorf("%w: %q must be a clean project-relative path without '.' or '..'", ErrInvalidProjectPath, raw)
		}
	}

	return normalized, filepath.FromSlash(normalized), nil
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}
