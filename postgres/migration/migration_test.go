package migration

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"testing/fstest"

	"github.com/pressly/goose/v3"
)

func TestProviderUsesExplicitSourcesOnly(t *testing.T) {
	goose.ResetGlobalMigrations()
	t.Cleanup(goose.ResetGlobalMigrations)

	global := goose.NewGoMigration(3, &goose.GoFunc{
		RunTx: func(context.Context, *sql.Tx) error { return nil },
	}, nil)
	if err := goose.SetGlobalMigrations(global); err != nil {
		t.Fatalf("register global migration: %v", err)
	}
	local := goose.NewGoMigration(2, &goose.GoFunc{
		RunTx: func(context.Context, *sql.Tx) error { return nil },
	}, nil)

	provider, err := newProvider(Config{
		DB: &sql.DB{},
		Migrations: fstest.MapFS{
			"0001_init.sql": &fstest.MapFile{Data: []byte("-- +goose Up\nSELECT 1;\n")},
		},
		GoMigrations: []*goose.Migration{local},
	})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	sources := provider.ListSources()
	if len(sources) != 2 {
		t.Fatalf("source count = %d, want 2", len(sources))
	}
	if sources[0].Version != 1 || sources[1].Version != 2 {
		t.Fatalf("source versions = [%d %d], want [1 2]", sources[0].Version, sources[1].Version)
	}
}

func TestVersionErrorPreservesKind(t *testing.T) {
	err := versionError(ErrSchemaAhead, 2, 1)
	if !errors.Is(err, ErrSchemaAhead) {
		t.Fatalf("version error does not wrap ErrSchemaAhead: %v", err)
	}
}

func TestRunToRejectsNonPositiveTarget(t *testing.T) {
	_, err := RunTo(context.Background(), Config{}, 0)
	if !errors.Is(err, ErrTargetInvalid) {
		t.Fatalf("RunTo error = %v, want ErrTargetInvalid", err)
	}
}

func TestTargetTotal(t *testing.T) {
	tests := []struct {
		name      string
		versions  []int64
		current   int64
		available int64
		target    int64
		want      int
		wantErr   error
	}{
		{name: "current", versions: []int64{9, 10, 11}, current: 9, available: 11, target: 9, want: 1},
		{name: "historical", versions: []int64{9, 10, 11}, current: 9, available: 11, target: 10, want: 2},
		{name: "latest", versions: []int64{9, 10, 11}, current: 9, available: 11, target: 11, want: 3},
		{name: "database ahead", versions: []int64{9, 10, 11}, current: 12, available: 11, target: 11, wantErr: ErrSchemaAhead},
		{name: "target behind", versions: []int64{9, 10, 11}, current: 10, available: 11, target: 9, wantErr: ErrTargetInvalid},
		{name: "target ahead", versions: []int64{9, 10, 11}, current: 9, available: 11, target: 12, wantErr: ErrTargetInvalid},
		{name: "target ceiling", versions: []int64{9, 11}, current: 9, available: 11, target: 10, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sources := make([]*goose.Source, 0, len(tt.versions))
			for _, version := range tt.versions {
				sources = append(sources, &goose.Source{Version: version})
			}

			got, err := targetTotal(sources, tt.current, tt.available, tt.target)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("targetTotal error = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("targetTotal = %d, want %d", got, tt.want)
			}
		})
	}
}
