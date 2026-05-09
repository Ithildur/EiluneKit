package logging

import (
	"context"
	"log/slog"
)

// Helper wraps *slog.Logger with Error(msg, err, attrs...) style calls.
// Helper 用 Error(msg, err, attrs...) 形式包装 *slog.Logger。
type Helper struct {
	logger *slog.Logger
}

// NewHelper returns a Helper for a base logger.
// Call NewHelper(base).Info(...) or NewHelper(base).Error(...).
// logger must not be nil.
// NewHelper 返回基于底层 logger 的 Helper。
// 调用 NewHelper(base).Info(...) 或 NewHelper(base).Error(...)。
// logger 不能为空。
func NewHelper(logger *slog.Logger) *Helper {
	if logger == nil {
		panic("logging: nil slog logger")
	}
	return &Helper{logger: logger}
}

// Logger returns the base logger.
// Use Logger() where an API requires *slog.Logger.
// Logger 返回底层 logger。
// 在某个 API 需要 *slog.Logger 时调用 Logger()。
func (h *Helper) Logger() *slog.Logger {
	if h == nil {
		return nil
	}
	return h.logger
}

// Debug logs msg, err, and attrs at debug level.
// Debug 以 debug 级别记录 msg、err 和 attrs。
func (h *Helper) Debug(msg string, err error, attrs ...slog.Attr) {
	h.log(slog.LevelDebug, msg, err, attrs...)
}

// Info logs msg, err, and attrs at info level.
// Info 以 info 级别记录 msg、err 和 attrs。
func (h *Helper) Info(msg string, err error, attrs ...slog.Attr) {
	h.log(slog.LevelInfo, msg, err, attrs...)
}

// Warn logs msg, err, and attrs at warn level.
// Warn 以 warn 级别记录 msg、err 和 attrs。
func (h *Helper) Warn(msg string, err error, attrs ...slog.Attr) {
	h.log(slog.LevelWarn, msg, err, attrs...)
}

// Error logs msg, err, and attrs at error level.
// Error 以 error 级别记录 msg、err 和 attrs。
func (h *Helper) Error(msg string, err error, attrs ...slog.Attr) {
	h.log(slog.LevelError, msg, err, attrs...)
}

// With returns a child helper with bound attrs.
// Call h.With(attrs...).Info(...) to reuse attrs across calls.
// With 返回绑定 attrs 的子 helper。
// 调用 h.With(attrs...).Info(...) 复用 attrs。
func (h *Helper) With(attrs ...slog.Attr) *Helper {
	if len(attrs) == 0 {
		return h
	}
	args := make([]any, 0, len(attrs))
	for _, attr := range attrs {
		args = append(args, attr)
	}
	return &Helper{logger: h.logger.With(args...)}
}

// Enabled reports whether a level is enabled.
// Call Enabled before building expensive attrs.
// Enabled 返回某个级别是否启用。
// 在构造代价高的 attrs 前调用 Enabled。
func (h *Helper) Enabled(level Level) bool {
	return h.logger.Enabled(context.Background(), level.SlogLevel())
}

func (h *Helper) log(level slog.Level, msg string, err error, attrs ...slog.Attr) {
	if err != nil {
		attrs = append(attrs, slog.Any("error", err))
	}
	h.logger.LogAttrs(context.Background(), level, msg, attrs...)
}
