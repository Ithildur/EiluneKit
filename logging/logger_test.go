package logging

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

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
