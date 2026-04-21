package static_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Ithildur/EiluneKit/appdir"
	"github.com/Ithildur/EiluneKit/http/static"
)

func TestResolveDirRejectsInvalidPath(t *testing.T) {
	t.Parallel()

	invalid := []string{
		"",
		".",
		"..",
		"./dist",
		"dist/../secret",
		"../secret",
		"/etc",
		"dist/",
		"dist//nested",
		`dist\..\secret`,
	}

	for _, raw := range invalid {
		raw := raw
		t.Run(filepath.Base(raw), func(t *testing.T) {
			t.Parallel()
			if _, err := static.ResolveDir(raw, static.Options{}); err == nil {
				t.Fatalf("expected %q to be rejected", raw)
			} else if !errors.Is(err, static.ErrInvalidProjectPath) {
				t.Fatalf("expected ErrInvalidProjectPath for %q, got %v", raw, err)
			}
		})
	}
}

func TestResolveDirUsesProjectRelativePath(t *testing.T) {
	home := t.TempDir()
	distDir := filepath.Join(home, "web", "dist")
	deployDir := filepath.Join(home, "deploy")
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		t.Fatalf("mkdir dist: %v", err)
	}
	if err := os.WriteFile(filepath.Join(distDir, "index.html"), []byte("ok"), 0o644); err != nil {
		t.Fatalf("write index.html: %v", err)
	}
	if err := os.MkdirAll(deployDir, 0o755); err != nil {
		t.Fatalf("mkdir deploy: %v", err)
	}

	const envVar = "EILUNEKIT_STATIC_TEST_HOME"
	t.Setenv(envVar, home)
	opts := static.Options{
		AppDir: appdir.Options{EnvVar: envVar},
	}

	dist, err := static.ResolveSPADir("dist", opts)
	if err != nil {
		t.Fatalf("resolve dist: %v", err)
	}
	if dist != distDir {
		t.Fatalf("expected dist dir %q, got %q", distDir, dist)
	}

	deploy, err := static.ResolveDir("deploy", opts)
	if err != nil {
		t.Fatalf("resolve deploy: %v", err)
	}
	if deploy != deployDir {
		t.Fatalf("expected deploy dir %q, got %q", deployDir, deploy)
	}
}

func TestResolveDirDoesNotFallbackToWorkingDirByDefault(t *testing.T) {
	home := t.TempDir()
	distDir := filepath.Join(home, "dist")
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		t.Fatalf("mkdir dist: %v", err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(home); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		if chdirErr := os.Chdir(oldWD); chdirErr != nil {
			t.Fatalf("restore wd: %v", chdirErr)
		}
	})

	if _, err := static.ResolveDir("dist", static.Options{}); err == nil {
		t.Fatal("expected ResolveDir without Development to ignore working dir")
	}
}

func TestResolveDirDevelopmentUsesWorkingDir(t *testing.T) {
	home := t.TempDir()
	distDir := filepath.Join(home, "dist")
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		t.Fatalf("mkdir dist: %v", err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(home); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		if chdirErr := os.Chdir(oldWD); chdirErr != nil {
			t.Fatalf("restore wd: %v", chdirErr)
		}
	})

	got, err := static.ResolveDir("dist", static.Options{Development: true})
	if err != nil {
		t.Fatalf("resolve dist: %v", err)
	}
	if got != distDir {
		t.Fatalf("expected dist dir %q, got %q", distDir, got)
	}
}

func TestResolveDirRejectsInvalidEnvVarWithoutWorkingDirFallback(t *testing.T) {
	envHome := t.TempDir()
	workingHome := t.TempDir()
	distDir := filepath.Join(workingHome, "dist")
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		t.Fatalf("mkdir dist: %v", err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(workingHome); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		if chdirErr := os.Chdir(oldWD); chdirErr != nil {
			t.Fatalf("restore wd: %v", chdirErr)
		}
	})

	const envVar = "EILUNEKIT_STATIC_TEST_INVALID_HOME"
	t.Setenv(envVar, envHome)
	_, err = static.ResolveDir("dist", static.Options{
		AppDir: appdir.Options{
			EnvVar: envVar,
			Markers: []string{
				"dist",
			},
			RequireDirMarkers: true,
		},
		Development: true,
	})
	if err == nil {
		t.Fatal("expected invalid env var to fail")
	}
	if !errors.Is(err, appdir.ErrEnvInvalid) {
		t.Fatalf("expected ErrEnvInvalid, got %v", err)
	}
}

func TestSPAHandlerFallsBackToIndex(t *testing.T) {
	home := t.TempDir()
	distDir := filepath.Join(home, "web", "dist")
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		t.Fatalf("mkdir dist: %v", err)
	}
	if err := os.WriteFile(filepath.Join(distDir, "index.html"), []byte("index"), 0o644); err != nil {
		t.Fatalf("write index.html: %v", err)
	}
	if err := os.WriteFile(filepath.Join(distDir, "app.js"), []byte("asset"), 0o644); err != nil {
		t.Fatalf("write app.js: %v", err)
	}

	const envVar = "EILUNEKIT_STATIC_TEST_HOME"
	t.Setenv(envVar, home)
	opts := static.Options{
		AppDir: appdir.Options{EnvVar: envVar},
	}

	handler, err := static.SPAHandler("dist", opts)
	if err != nil {
		t.Fatalf("spa handler: %v", err)
	}

	assetReq := httptest.NewRequest(http.MethodGet, "/app.js", nil)
	assetRec := httptest.NewRecorder()
	handler.ServeHTTP(assetRec, assetReq)
	if assetRec.Code != http.StatusOK {
		t.Fatalf("expected asset status %d, got %d", http.StatusOK, assetRec.Code)
	}
	if got := assetRec.Body.String(); got != "asset" {
		t.Fatalf("expected asset body %q, got %q", "asset", got)
	}

	spaReq := httptest.NewRequest(http.MethodGet, "/dashboard/settings", nil)
	spaRec := httptest.NewRecorder()
	handler.ServeHTTP(spaRec, spaReq)
	if spaRec.Code != http.StatusOK {
		t.Fatalf("expected spa status %d, got %d", http.StatusOK, spaRec.Code)
	}
	if got := spaRec.Body.String(); got != "index" {
		t.Fatalf("expected index fallback body %q, got %q", "index", got)
	}
}
