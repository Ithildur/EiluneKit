package appdir_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Ithildur/EiluneKit/appdir"
)

func TestDiscoverHomeRejectsInvalidEnvVar(t *testing.T) {
	home := t.TempDir()
	const envVar = "EILUNEKIT_APPDIR_TEST_HOME"
	t.Setenv(envVar, home)

	_, err := appdir.DiscoverHome(appdir.Options{
		EnvVar: envVar,
		Markers: []string{
			"configs",
		},
		RequireDirMarkers: true,
	})
	if err == nil {
		t.Fatal("expected invalid env var to fail")
	}
	if !errors.Is(err, appdir.ErrEnvInvalid) {
		t.Fatalf("expected ErrEnvInvalid, got %v", err)
	}
}

func TestDiscoverHomeUsesWorkingDirWhenRequested(t *testing.T) {
	home := t.TempDir()
	configs := filepath.Join(home, "configs")
	if err := os.MkdirAll(configs, 0o755); err != nil {
		t.Fatalf("mkdir configs: %v", err)
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

	got, err := appdir.DiscoverHome(appdir.Options{
		Markers: []string{
			"configs",
		},
		RequireDirMarkers: true,
		Sources:           appdir.SourceWorkingDir,
	})
	if err != nil {
		t.Fatalf("discover home: %v", err)
	}
	if got != home {
		t.Fatalf("expected home %q, got %q", home, got)
	}
}
