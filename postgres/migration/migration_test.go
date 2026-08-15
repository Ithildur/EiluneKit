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
