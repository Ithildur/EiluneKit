package middleware

import (
	"bytes"
	"encoding/json/v2"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

func TestAccessLogRequests(t *testing.T) {
	var output bytes.Buffer
	var level slog.LevelVar
	logger := slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: &level}))
	for _, test := range []struct {
		name       string
		level      slog.Level
		status     int
		incomingID string
		requestID  func(http.Handler) http.Handler
		logged     bool
	}{
		{"filtered", slog.LevelError, 204, "test-request", chimiddleware.RequestID, false},
		{"success", slog.LevelInfo, 204, "test-request", chimiddleware.RequestID, true},
		{"client error", slog.LevelWarn, 400, "test-request", chimiddleware.RequestID, true},
		{"server error", slog.LevelError, 500, "test-request", chimiddleware.RequestID, true},
		{"generated ID", slog.LevelInfo, 204, "", RequestID, true},
		{"incoming ID", slog.LevelInfo, 204, "upstream-id", RequestID, true},
		{"implicit status", slog.LevelInfo, 0, "", RequestID, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			output.Reset()
			var id string
			h := test.requestID(AccessLog(AccessLogOptions{Logger: logger})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				id = RequestIDFromContext(r.Context())
				if test.status != 0 {
					w.WriteHeader(test.status)
				}
			})))
			level.Set(test.level)
			r := httptest.NewRequest(http.MethodGet, "/resource", nil)
			r.Header.Set("User-Agent", "test-agent")
			r.Header.Set("X-Request-Id", test.incomingID)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			wantStatus := test.status
			if wantStatus == 0 {
				wantStatus = http.StatusOK
			}
			if w.Code != wantStatus {
				t.Fatalf("response status %d, want %d", w.Code, wantStatus)
			}
			if !test.logged {
				if output.Len() != 0 {
					t.Fatalf("unexpected log: %s", output.String())
				}
				return
			}
			var record struct {
				Status    int    `json:"status"`
				Method    string `json:"method"`
				Path      string `json:"path"`
				RequestID string `json:"request_id"`
				RemoteIP  string `json:"remote_ip"`
				UserAgent string `json:"user_agent"`
				Aborted   *bool  `json:"aborted"`
			}
			if err := json.Unmarshal(output.Bytes(), &record); err != nil {
				t.Fatal(err)
			}
			if id == "" || record.RequestID != id || (test.incomingID != "" && id != test.incomingID) {
				t.Fatalf("incoming %q, context %q, log %q", test.incomingID, id, record.RequestID)
			}
			if record.Status != wantStatus || record.Method != "GET" || record.Path != "/resource" ||
				record.RemoteIP != "192.0.2.1" || record.UserAgent != "test-agent" || record.Aborted != nil {
				t.Fatalf("unexpected log: %s", output.String())
			}
		})
	}
}

func TestAccessLogAbortedRequests(t *testing.T) {
	failure := errors.New("handler panic")
	for _, test := range []struct {
		name    string
		write   func(http.ResponseWriter)
		panic   error
		status  int
		skip    bool
		minimum slog.Level
	}{
		{name: "before headers", panic: http.ErrAbortHandler},
		{name: "after headers", write: func(w http.ResponseWriter) { w.WriteHeader(202) }, panic: failure, status: 202},
		{name: "after body", write: func(w http.ResponseWriter) { _, _ = io.WriteString(w, "partial") }, panic: failure, status: 200},
		{name: "after flush", write: func(w http.ResponseWriter) { _ = http.NewResponseController(w).Flush() }, panic: http.ErrAbortHandler, status: 200},
		{name: "informational", write: func(w http.ResponseWriter) { w.WriteHeader(103) }, panic: failure},
		{name: "skipped", panic: http.ErrAbortHandler, skip: true},
		{name: "filtered", panic: http.ErrAbortHandler, minimum: slog.LevelError + 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			skipCalled := false
			h := AccessLog(AccessLogOptions{
				Logger:   slog.New(slog.NewJSONHandler(&output, nil)),
				MinLevel: test.minimum,
				Skip: func(_ *http.Request, status int) bool {
					skipCalled = true
					if status != test.status {
						t.Errorf("Skip status %d, want %d", status, test.status)
					}
					return test.skip
				},
			})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if test.write != nil {
					test.write(w)
				}
				panic(test.panic)
			}))
			func() {
				defer func() {
					if got := recover(); got != test.panic {
						t.Errorf("panic changed: %v", got)
					}
				}()
				h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
			}()
			if !skipCalled {
				t.Fatal("aborted request bypassed logging")
			}
			if test.skip || test.minimum > slog.LevelError {
				if output.Len() != 0 {
					t.Fatalf("unexpected log: %s", output.String())
				}
				return
			}
			var record struct {
				Status  int    `json:"status"`
				Aborted bool   `json:"aborted"`
				Level   string `json:"level"`
			}
			if err := json.Unmarshal(output.Bytes(), &record); err != nil {
				t.Fatal(err)
			}
			if record.Status != test.status || !record.Aborted || record.Level != "ERROR" {
				t.Fatalf("unexpected aborted log: %s", output.String())
			}
		})
	}
}
