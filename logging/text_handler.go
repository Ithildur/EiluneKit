package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"
)

type textHandler struct {
	level       slog.Level
	timeFormat  string
	writer      io.Writer
	mu          *sync.Mutex
	attrs       []slog.Attr
	groupPrefix string
}

func newTextHandler(w io.Writer, level Level, timeFormat string) slog.Handler {
	return &textHandler{
		level:      level.SlogLevel(),
		timeFormat: timeFormat,
		writer:     w,
		mu:         &sync.Mutex{},
	}
}

func (h *textHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *textHandler) Handle(_ context.Context, r slog.Record) error {
	ts := r.Time
	if ts.IsZero() {
		ts = time.Now()
	}
	line := strings.Builder{}
	line.WriteString(ts.Local().Format(h.timeFormat))
	line.WriteString(" [")
	line.WriteString(strings.ToUpper(r.Level.String()))
	line.WriteString("] ")
	line.WriteString(r.Message)

	for _, a := range h.attrs {
		h.appendAttr(&line, h.groupPrefix, a)
	}
	r.Attrs(func(a slog.Attr) bool {
		h.appendAttr(&line, h.groupPrefix, a)
		return true
	})
	line.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.writer, line.String())
	return err
}

func (h *textHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := *h
	next.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &next
}

func (h *textHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	next := *h
	if next.groupPrefix == "" {
		next.groupPrefix = name + "."
	} else {
		next.groupPrefix = next.groupPrefix + name + "."
	}
	return &next
}

func (h *textHandler) appendAttr(line *strings.Builder, prefix string, a slog.Attr) {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return
	}
	if a.Value.Kind() == slog.KindGroup {
		groupPrefix := prefix
		if a.Key != "" {
			groupPrefix = groupPrefix + a.Key + "."
		}
		for _, child := range a.Value.Group() {
			h.appendAttr(line, groupPrefix, child)
		}
		return
	}
	if a.Key == "" {
		return
	}
	key := prefix + a.Key
	line.WriteByte(' ')
	line.WriteString(key)
	line.WriteByte('=')
	line.WriteString(formatValue(a.Value, h.timeFormat))
}

func formatValue(v slog.Value, timeFormat string) string {
	switch v.Kind() {
	case slog.KindString:
		return formatValueString(v.String())
	case slog.KindTime:
		return formatValueString(v.Time().Local().Format(timeFormat))
	case slog.KindDuration:
		return formatValueString(v.Duration().String())
	case slog.KindBool:
		return strconv.FormatBool(v.Bool())
	case slog.KindInt64:
		return strconv.FormatInt(v.Int64(), 10)
	case slog.KindUint64:
		return strconv.FormatUint(v.Uint64(), 10)
	case slog.KindFloat64:
		return strconv.FormatFloat(v.Float64(), 'f', -1, 64)
	case slog.KindAny:
		if err, ok := v.Any().(error); ok {
			return formatValueString(err.Error())
		}
		if v.Any() == nil {
			return "null"
		}
		return formatValueString(fmt.Sprint(v.Any()))
	default:
		return formatValueString(fmt.Sprint(v.Any()))
	}
}

func needsQuoting(s string) bool {
	if s == "" {
		return true
	}
	for _, r := range s {
		if r <= ' ' || r == '=' || r == '"' {
			return true
		}
	}
	return false
}

func formatValueString(s string) string {
	if needsQuoting(s) {
		return strconv.Quote(s)
	}
	return s
}
