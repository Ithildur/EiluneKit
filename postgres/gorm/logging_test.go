package gorm

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	gormcore "gorm.io/gorm"
)

func TestNewLoggerHidesQueryParamsByDefault(t *testing.T) {
	var out bytes.Buffer
	base := slog.New(slog.NewTextHandler(&out, nil))
	log := NewLogger(LogOptions{Logger: base, Level: slog.LevelInfo})

	filter, ok := log.(gormcore.ParamsFilter)
	if !ok {
		t.Fatal("expected logger to expose query parameter filtering")
	}

	_, params := filter.ParamsFilter(context.Background(), "select * from users where email = ?", "admin@example.com")
	if params != nil {
		t.Fatalf("expected query params to be hidden by default")
	}
}

func TestNewLoggerCanIncludeQueryParams(t *testing.T) {
	var out bytes.Buffer
	base := slog.New(slog.NewTextHandler(&out, nil))
	log := NewLogger(LogOptions{
		Logger:             base,
		Level:              slog.LevelInfo,
		IncludeQueryParams: true,
	})

	filter, ok := log.(gormcore.ParamsFilter)
	if !ok {
		t.Fatal("expected logger to expose query parameter filtering")
	}

	_, params := filter.ParamsFilter(context.Background(), "select * from users where email = ?", "admin@example.com")
	if len(params) != 1 || params[0] != "admin@example.com" {
		t.Fatalf("expected query params to be preserved")
	}
}
