// Package appdir discovers the application home directory.
// Package appdir 发现应用主目录。
package appdir

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrNoCandidates = errors.New("appdir: no candidates available")
	ErrNotFound     = errors.New("appdir: cannot discover home directory")
	ErrEnvInvalid   = errors.New("appdir: env home is invalid")
)

// CandidateSource selects where DiscoverHome searches.
// CandidateSource 选择 DiscoverHome 的搜索来源。
type CandidateSource uint8

const (
	SourceEnvVar CandidateSource = 1 << iota
	SourceExecutable
	SourceWorkingDir
)

const defaultSources = SourceEnvVar | SourceExecutable | SourceWorkingDir

// Options configures DiscoverHome.
// Options 配置 DiscoverHome。
type Options struct {
	EnvVar            string
	Markers           []string
	RequireDirMarkers bool
	Sources           CandidateSource
}

// DiscoverHome returns the first candidate directory that matches Markers.
// DiscoverHome 返回第一个匹配 Markers 的候选目录。
//
// Example / 示例:
//
//	home, _ := appdir.DiscoverHome(appdir.Options{EnvVar: "APP_HOME", Markers: []string{"configs"}})
func DiscoverHome(opts Options) (string, error) {
	sources := effectiveSources(opts.Sources)
	if sources&SourceEnvVar != 0 && strings.TrimSpace(opts.EnvVar) != "" {
		if raw, ok := os.LookupEnv(opts.EnvVar); ok {
			dir := strings.TrimSpace(raw)
			if dir == "" || !isHome(filepath.Clean(dir), opts.Markers, opts.RequireDirMarkers) {
				return "", fmt.Errorf("%w: %s=%q", ErrEnvInvalid, opts.EnvVar, raw)
			}
			return filepath.Abs(dir)
		}
	}

	candidates := collectCandidates(sources)
	if len(candidates) == 0 {
		return "", ErrNoCandidates
	}

	for _, dir := range candidates {
		if isHome(dir, opts.Markers, opts.RequireDirMarkers) {
			return filepath.Abs(dir)
		}
	}

	return "", ErrNotFound
}

func collectCandidates(sources CandidateSource) []string {
	var out []string
	seen := make(map[string]struct{})

	add := func(p string) {
		if p == "" {
			return
		}
		p = filepath.Clean(p)
		if p == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}

	if sources&SourceExecutable != 0 {
		if exe, err := os.Executable(); err == nil && exe != "" {
			if resolved, resolveErr := filepath.EvalSymlinks(exe); resolveErr == nil && resolved != "" {
				exe = resolved
			}
			exeDir := filepath.Dir(exe)
			add(exeDir)
			if filepath.Base(exeDir) == "bin" {
				add(filepath.Dir(exeDir))
			}
		}
	}

	if sources&SourceWorkingDir != 0 {
		if wd, err := os.Getwd(); err == nil && wd != "" {
			add(wd)
		}
	}

	return out
}

func effectiveSources(sources CandidateSource) CandidateSource {
	if sources == 0 {
		return defaultSources
	}
	return sources
}

func isHome(dir string, markers []string, requireDir bool) bool {
	if dir == "" {
		return false
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}

	if len(markers) == 0 {
		return true
	}

	for _, m := range markers {
		if markerExists(dir, m, requireDir) {
			return true
		}
	}
	return false
}

func markerExists(base, marker string, requireDir bool) bool {
	if marker == "" {
		return false
	}
	full := filepath.Join(base, marker)
	info, err := os.Stat(full)
	if err != nil {
		return false
	}
	if !requireDir {
		return true
	}
	return info.IsDir()
}
