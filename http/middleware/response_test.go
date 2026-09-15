package middleware

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fileResponseWriter struct {
	*httptest.ResponseRecorder
	readFrom bool
}

func (w *fileResponseWriter) ReadFrom(src io.Reader) (int64, error) {
	w.readFrom = true
	return io.Copy(w.ResponseRecorder, src)
}

func TestObservedFileResponse(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := AccessLog(AccessLogOptions{Logger: logger})(Recover(RecoverOptions{Logger: logger})(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.ServeContent(w, r, "file.txt", time.Time{}, bytes.NewReader([]byte("file content")))
		}),
	))
	w := &fileResponseWriter{ResponseRecorder: httptest.NewRecorder()}
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/file.txt", nil))
	if w.Code != http.StatusOK || w.Body.String() != "file content" || !w.readFrom {
		t.Fatalf("file response: %d %q, ReadFrom forwarded: %t", w.Code, w.Body.String(), w.readFrom)
	}
}

func BenchmarkObservedFileResponse(b *testing.B) {
	dir := b.TempDir()
	const size = 8 << 20
	if err := os.WriteFile(filepath.Join(dir, "file.bin"), bytes.Repeat([]byte("x"), size), 0o600); err != nil {
		b.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, observe := range []bool{false, true} {
		name := "plain"
		var h http.Handler = http.FileServer(http.Dir(dir))
		if observe {
			name = "accesslog+recover"
			h = AccessLog(AccessLogOptions{Logger: logger})(Recover(RecoverOptions{Logger: logger})(h))
		}
		b.Run(name, func(b *testing.B) {
			srv := httptest.NewServer(h)
			defer srv.Close()
			client := srv.Client()
			client.Timeout = 5 * time.Second
			b.SetBytes(size)
			b.ReportAllocs()
			for b.Loop() {
				resp, err := client.Get(srv.URL + "/file.bin")
				if err != nil {
					b.Fatal(err)
				}
				n, err := io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				if err != nil || resp.StatusCode != http.StatusOK || n != size {
					b.Fatalf("file response: %d, bytes %d, error %v", resp.StatusCode, n, err)
				}
			}
		})
	}
}
