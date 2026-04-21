package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"
)

// Level is the log verbosity.
// Level 表示日志级别。
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	default:
		return "info"
	}
}

// SlogLevel returns the corresponding slog level.
// SlogLevel 返回对应的 slog 级别。
func (l Level) SlogLevel() slog.Level {
	switch l {
	case LevelDebug:
		return slog.LevelDebug
	case LevelInfo:
		return slog.LevelInfo
	case LevelWarn:
		return slog.LevelWarn
	case LevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Format is the log output format.
// Format 表示日志输出格式。
type Format string

const (
	FormatText Format = "text"
	FormatJSON Format = "json"
)

const defaultTimeFormat = time.RFC3339

// Options configures New.
// Options 配置 New。
type Options struct {
	Level      Level
	Format     Format
	Writer     io.Writer
	TimeFormat string
	AddSource  bool
}

// ParseLevel parses `debug`, `info`, `warn`, or `error`.
// ParseLevel 解析 `debug`、`info`、`warn` 或 `error`。
func ParseLevel(raw string) (Level, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "info":
		return LevelInfo, nil
	case "debug":
		return LevelDebug, nil
	case "warn", "warning":
		return LevelWarn, nil
	case "error":
		return LevelError, nil
	default:
		return LevelInfo, fmt.Errorf("logging: unknown level %q", raw)
	}
}

// ParseFormat parses `text` or `json`.
// ParseFormat 解析 `text` 或 `json`。
func ParseFormat(raw string) (Format, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "text":
		return FormatText, nil
	case "json":
		return FormatJSON, nil
	default:
		return FormatText, fmt.Errorf("logging: unknown format %q", raw)
	}
}

// New returns a base logger for APIs that accept *slog.Logger.
// Call New(Options{...}) to build the base logger.
// New 返回供接收 *slog.Logger 的 API 使用的底层 logger。
// 调用 New(Options{...}) 构建底层 logger。
func New(opts Options) *slog.Logger {
	writer := opts.Writer
	if writer == nil {
		writer = os.Stdout
	}
	if !isValidLevel(opts.Level) {
		opts.Level = LevelInfo
	}
	if !isValidFormat(opts.Format) {
		opts.Format = FormatText
	}
	if opts.TimeFormat == "" {
		opts.TimeFormat = defaultTimeFormat
	}

	var handler slog.Handler
	switch opts.Format {
	case FormatJSON:
		handler = slog.NewJSONHandler(writer, &slog.HandlerOptions{
			Level:     opts.Level.SlogLevel(),
			AddSource: opts.AddSource,
			ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
				if a.Key == slog.TimeKey {
					t := a.Value.Time()
					if !t.IsZero() {
						a.Value = slog.StringValue(t.Local().Format(opts.TimeFormat))
					}
				}
				return a
			},
		})
	default:
		handler = newTextHandler(writer, opts.Level, opts.TimeFormat)
	}

	return slog.New(handler)
}

func isValidLevel(level Level) bool {
	switch level {
	case LevelDebug, LevelInfo, LevelWarn, LevelError:
		return true
	default:
		return false
	}
}

func isValidFormat(format Format) bool {
	switch format {
	case FormatText, FormatJSON:
		return true
	default:
		return false
	}
}
