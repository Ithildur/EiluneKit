package logging

import (
	"bytes"
	"encoding/json/v2"
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestTextLogEscapesRecordFields(t *testing.T) {
	for _, value := range []string{"line\nforged", "line\rforged", "tab\tvalue", "escape\x1b[31m", "delete\x7f", "next\u0085line", "line\u2028separator"} {
		t.Run(value, func(t *testing.T) {
			var out bytes.Buffer
			logger := New(Options{Writer: &out})
			logger.WithGroup(value).With(slog.String(value, value)).Info(value, slog.String(value, value))
			line := out.String()
			if strings.Count(line, "\n") != 1 || strings.ContainsAny(line, "\r\t\x1b\x7f\u0085\u2028") {
				t.Fatalf("control characters escaped the record: %q", line)
			}
			attr := strconv.Quote(value+"."+value) + "=" + strconv.Quote(value)
			want := "[INFO] " + strconv.Quote(value) + " " + attr + " " + attr + "\n"
			if !strings.HasSuffix(line, want) {
				t.Fatalf("unexpected output: got %q, want suffix %q", line, want)
			}
		})
	}
	var out bytes.Buffer
	logger := New(Options{Writer: &out})
	logger.Info("user saved", "user_id", "user-1", "bad key=\"name\"", "value")
	want := "[INFO] user saved user_id=user-1 " + strconv.Quote("bad key=\"name\"") + "=value\n"
	if !strings.HasSuffix(out.String(), want) {
		t.Fatalf("unexpected printable output: %q", out.String())
	}
}

func TestDefaultTimeFormatIncludesLocalZoneText(t *testing.T) {
	oldLocal := time.Local
	time.Local = time.FixedZone("HKT", 8*60*60)
	t.Cleanup(func() {
		time.Local = oldLocal
	})

	var buf bytes.Buffer
	logger := New(Options{
		Format: FormatText,
		Writer: &buf,
	})
	logger.Info("hello")

	line := buf.String()
	if !strings.Contains(line, "+08:00") {
		t.Fatalf("expected local offset in log line, got %q", line)
	}
	if strings.Contains(line, "HKT") {
		t.Fatalf("expected RFC3339 log line without zone abbreviation, got %q", line)
	}
}

func TestDefaultTimeFormatIncludesLocalZoneJSON(t *testing.T) {
	oldLocal := time.Local
	time.Local = time.FixedZone("HKT", 8*60*60)
	t.Cleanup(func() {
		time.Local = oldLocal
	})

	var buf bytes.Buffer
	logger := New(Options{
		Format: FormatJSON,
		Writer: &buf,
	})
	logger.Info("hello")

	var payload map[string]any
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal log line: %v", err)
	}

	raw, ok := payload["time"].(string)
	if !ok {
		t.Fatalf("expected time string, got %#v", payload["time"])
	}
	if !strings.Contains(raw, "+08:00") {
		t.Fatalf("expected local offset in json time, got %q", raw)
	}
	if strings.Contains(raw, "HKT") {
		t.Fatalf("expected RFC3339 json time without zone abbreviation, got %q", raw)
	}
}
