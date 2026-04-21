package gorm

import (
	"context"
	"fmt"
	"log/slog"

	"gorm.io/gorm/logger"
)

// LogOptions configures NewLogger.
// LogOptions 配置 NewLogger。
type LogOptions struct {
	Logger               *slog.Logger
	Level                slog.Level
	Disabled             bool
	IgnoreRecordNotFound bool
}

// NewLogger returns a GORM logger backed by slog.
// Call NewLogger(LogOptions{Logger: base, Level: slog.LevelInfo}).
// NewLogger 返回基于 slog 的 GORM logger。
// 调用 NewLogger(LogOptions{Logger: base, Level: slog.LevelInfo})。
func NewLogger(opts LogOptions) logger.Interface {
	if opts.Disabled || opts.Logger == nil {
		return logger.Discard
	}
	writer := logWriter{
		logger: opts.Logger,
		level:  opts.Level,
	}
	return logger.New(writer, logger.Config{
		IgnoreRecordNotFoundError: opts.IgnoreRecordNotFound,
		LogLevel:                  toGormLevel(opts.Level),
	})
}

type logWriter struct {
	logger *slog.Logger
	level  slog.Level
}

func (w logWriter) Printf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	w.logger.Log(context.Background(), w.level, msg)
}

func toGormLevel(level slog.Level) logger.LogLevel {
	switch {
	case level <= slog.LevelInfo:
		return logger.Info
	case level <= slog.LevelWarn:
		return logger.Warn
	default:
		return logger.Error
	}
}
