package middleware

import (
	"bytes"
	"encoding/json/v2"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

func TestAccessLogHonorsDynamicLevels(t *testing.T) {
	var output bytes.Buffer
	var level slog.LevelVar
	logger := slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: &level}))
	status := http.StatusNoContent
	h := chimiddleware.RequestID(AccessLog(AccessLogOptions{Logger: logger})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) })))
	for _, test := range []struct {
		level  slog.Level
		status int
		logged bool
	}{
		{slog.LevelError, http.StatusNoContent, false},
		{slog.LevelInfo, http.StatusNoContent, true},
		{slog.LevelWarn, http.StatusBadRequest, true},
		{slog.LevelError, http.StatusInternalServerError, true},
	} {
		output.Reset()
		level.Set(test.level)
		status = test.status
		r := httptest.NewRequest(http.MethodGet, "/resource", nil)
		r.Header.Set("User-Agent", "test-agent")
		r.Header.Set("X-Request-Id", "test-request")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("response status: %d; want %d", w.Code, status)
		}
		if !test.logged {
			if output.Len() != 0 {
				t.Fatalf("unexpected log: %s", output.String())
			}
			continue
		}
		var record struct {
			Status    int    `json:"status"`
			Method    string `json:"method"`
			Path      string `json:"path"`
			RequestID string `json:"request_id"`
			RemoteIP  string `json:"remote_ip"`
			UserAgent string `json:"user_agent"`
		}
		if err := json.Unmarshal(output.Bytes(), &record); err != nil {
			t.Fatal(err)
		}
		if record.Status != status || record.Method != http.MethodGet || record.Path != "/resource" || record.RequestID != "test-request" || record.RemoteIP != "192.0.2.1" || record.UserAgent != "test-agent" {
			t.Fatalf("unexpected log: %s", output.String())
		}
	}
}
