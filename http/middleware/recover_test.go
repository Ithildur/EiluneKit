package middleware

import (
	"bufio"
	"bytes"
	"encoding/json/v2"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Ithildur/EiluneKit/http/response"
)

func TestRecoverLogsRequestIDAndUsesInjectedResponse(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	for _, custom := range []bool{false, true} {
		logs.Reset()
		opts := RecoverOptions{Logger: logger}
		if custom {
			opts.OnPanic = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = io.WriteString(w, "unavailable")
			})
		}
		h := RequestID(Recover(opts)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if RequestIDFromContext(r.Context()) != "request-42" {
				t.Error("request ID not propagated")
			}
			panic("private details")
		})))
		r := httptest.NewRequest("GET", "/panic", nil)
		r.Header.Set("X-Request-Id", "request-42")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		want := 500
		if custom {
			want = 503
		}
		if w.Code != want || strings.Contains(w.Body.String(), "private details") {
			t.Fatalf("panic response: %d %s", w.Code, w.Body.String())
		}
		var record struct {
			Panic     string `json:"panic"`
			Stack     string `json:"stack"`
			RequestID string `json:"request_id"`
		}
		if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
			t.Fatal(err)
		}
		if record.Panic != "private details" || record.Stack == "" || record.RequestID != "request-42" {
			t.Fatalf("panic log: %s", logs.String())
		}
	}
}

func TestRecoverAbortsStartedResponses(t *testing.T) {
	for _, start := range []string{"header", "body", "flush", "abort"} {
		t.Run(start, func(t *testing.T) {
			var logs bytes.Buffer
			h := Recover(RecoverOptions{
				Logger: slog.New(slog.NewJSONHandler(&logs, nil)),
				OnPanic: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
					t.Error("panic response ran after response started")
				}),
			})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch start {
				case "header":
					w.WriteHeader(202)
				case "body":
					_, _ = io.WriteString(w, "partial")
				case "flush":
					if err := http.NewResponseController(w).Flush(); err != nil {
						t.Fatal(err)
					}
				case "abort":
					panic(http.ErrAbortHandler)
				}
				panic("failure")
			}))
			defer func() {
				if value := recover(); value != http.ErrAbortHandler {
					t.Errorf("panic = %v, want ErrAbortHandler", value)
				}
				if start == "abort" && logs.Len() != 0 {
					t.Errorf("abort handler was logged: %s", logs.String())
				}
			}()
			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
		})
	}
}

func TestRecoverAfterEmptyCopy(t *testing.T) {
	h := Recover(RecoverOptions{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Hide Reader.WriteTo so io.Copy exercises the response writer's ReadFrom.
		// 隐藏 Reader.WriteTo，使 io.Copy 使用响应 writer 的 ReadFrom。
		_, _ = io.Copy(w, struct{ io.Reader }{strings.NewReader("")})
		panic("before response")
	}))
	srv := httptest.NewServer(h)
	defer srv.Close()
	client := srv.Client()
	client.Timeout = 5 * time.Second
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 500 {
		t.Fatalf("empty copy prevented recovery: %d", resp.StatusCode)
	}
}

func TestRecoverReplacesContentLength(t *testing.T) {
	h := Recover(RecoverOptions{
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		OnPanic: response.InternalServerError(),
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strings.TrimPrefix(r.URL.Path, "/"))
		w.Header().Set("Cache-Control", "no-store")
		panic("before response")
	}))
	srv := httptest.NewServer(h)
	defer srv.Close()
	client := srv.Client()
	client.Timeout = 5 * time.Second
	for _, length := range []string{"1", "1024"} {
		resp, err := client.Get(srv.URL + "/" + length)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusInternalServerError ||
			string(body) != `{"code":"internal_error","message":"internal server error"}` ||
			resp.Header.Get("Cache-Control") != "no-store" {
			t.Errorf("old length %s: status %d, body %q, headers %v, error %v", length, resp.StatusCode, body, resp.Header, err)
		}
	}
}

func TestRecoverClosesHijackedConnectionOnPanic(t *testing.T) {
	done := make(chan struct{})
	h := Compress(CompressOptions{})(Recover(RecoverOptions{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		OnPanic: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Error("attempted HTTP response after hijack")
		}),
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(done)
		_, rw, err := http.NewResponseController(w).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: test\r\n\r\n")
		_ = rw.Flush()
		panic("after hijack")
	})))
	h = AccessLog(AccessLogOptions{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})(h)
	srv := httptest.NewServer(h)
	defer srv.Close()
	conn, err := net.DialTimeout("tcp", srv.Listener.Addr().String(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	_, _ = io.WriteString(conn, "GET / HTTP/1.1\r\nHost: example\r\nConnection: Upgrade\r\nUpgrade: test\r\nAccept-Encoding: gzip\r\n\r\n")
	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, nil)
	if err != nil || resp.StatusCode != 101 {
		t.Fatalf("upgrade response: %v, error %v", resp, err)
	}
	if _, err := reader.ReadByte(); err != io.EOF {
		t.Fatalf("hijacked connection not closed: %v", err)
	}
	<-done
}
