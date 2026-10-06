package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

type textHandler struct {
	level       slog.Level
	timeFormat  string
	addSource   bool
	writer      io.Writer
	mu          *sync.Mutex
	attrs       []textAttr
	groupPrefix string
}

type textAttr struct {
	prefix string
	attr   slog.Attr
}

func newTextHandler(w io.Writer, level Level, timeFormat string, addSource bool) slog.Handler {
	return &textHandler{
		level:      level.SlogLevel(),
		timeFormat: timeFormat,
		addSource:  addSource,
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
	line.WriteString(formatMessage(r.Message))

	if h.addSource {
		if source := r.Source(); source != nil {
			h.appendAttr(&line, "", slog.String(slog.SourceKey, source.File+":"+strconv.Itoa(source.Line)))
		}
	}

	for _, a := range h.attrs {
		h.appendAttr(&line, a.prefix, a.attr)
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
	next.attrs = slices.Clone(h.attrs)
	for _, a := range attrs {
		next.attrs = append(next.attrs, textAttr{prefix: h.groupPrefix, attr: a})
	}
	return &next
}

func (h *textHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	next := *h
	next.groupPrefix += name + "."
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
	line.WriteString(formatString(key))
	line.WriteByte('=')
	line.WriteString(formatValue(a.Value, h.timeFormat))
}

func formatValue(v slog.Value, timeFormat string) string {
	switch v.Kind() {
	case slog.KindString:
		return formatString(v.String())
	case slog.KindTime:
		return formatString(v.Time().Local().Format(timeFormat))
	case slog.KindDuration:
		return formatString(v.Duration().String())
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
			return formatString(err.Error())
		}
		if v.Any() == nil {
			return "null"
		}
		return formatString(fmt.Sprint(v.Any()))
	default:
		return formatString(fmt.Sprint(v.Any()))
	}
}

func needsQuoting(s string) bool {
	if s == "" {
		return true
	}
	for _, r := range s {
		if r <= ' ' || r == '=' || r == '"' || !strconv.IsPrint(r) {
			return true
		}
	}
	return false
}

func formatString(s string) string {
	if needsQuoting(s) {
		return strconv.Quote(s)
	}
	return s
}

func formatMessage(s string) string {
	for _, r := range s {
		if !strconv.IsPrint(r) {
			return strconv.Quote(s)
		}
	}
	return s
}
