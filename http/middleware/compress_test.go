package middleware

import (
	"compress/gzip"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCompressNegotiation(t *testing.T) {
	body := strings.Repeat("response ", 200)
	for _, test := range []struct {
		name, accept, method, contentType, encoding string
		options                                     CompressOptions
		compressed                                  bool
	}{
		{"gzip", "gzip", "GET", "text/plain", "", CompressOptions{}, true},
		{"disabled by client", "gzip;q=0", "GET", "text/plain", "", CompressOptions{}, false},
		{"unaccepted", "br", "GET", "text/plain", "", CompressOptions{}, false},
		{"head", "gzip", "HEAD", "text/plain", "", CompressOptions{}, false},
		{"already encoded", "gzip", "GET", "text/plain", "br", CompressOptions{}, false},
		{"below threshold", "gzip", "GET", "text/plain", "", CompressOptions{MinSize: len(body) + 1}, false},
		{"type excluded", "gzip", "GET", "text/plain", "", CompressOptions{ContentTypes: []string{"application/json"}}, false},
		{"type included", "gzip", "GET", "text/plain; charset=utf-8", "", CompressOptions{Level: gzip.BestSpeed, ContentTypes: []string{"text/plain"}}, true},
		{"default type filter", "gzip", "GET", "image/jpeg", "", CompressOptions{}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := Compress(test.options)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", test.contentType)
				w.Header().Set("ETag", `"original"`)
				if test.encoding != "" {
					w.Header().Set("Content-Encoding", test.encoding)
				}
				_, _ = io.WriteString(w, body)
			}))
			r := httptest.NewRequest(test.method, "/", nil)
			r.Header.Set("Accept-Encoding", test.accept)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if got := w.Header().Get("Content-Encoding"); (got == "gzip") != test.compressed {
				t.Fatalf("encoding %q, compressed %t", got, test.compressed)
			}
			var reader io.Reader = w.Body
			if test.compressed {
				gz, err := gzip.NewReader(w.Body)
				if err != nil {
					t.Fatal(err)
				}
				defer gz.Close()
				reader = gz
				if w.Header().Get("ETag") != "" {
					t.Error("compressed representation retained original ETag")
				}
			}
			got, err := io.ReadAll(reader)
			if err != nil || string(got) != body {
				t.Fatalf("body mismatch, error %v", err)
			}
			if !strings.Contains(strings.Join(w.Header().Values("Vary"), ","), "Accept-Encoding") {
				t.Error("missing encoding Vary")
			}
		})
	}
}

func TestCompressStreamsBeforeHandlerReturns(t *testing.T) {
	finish := make(chan struct{})
	first := strings.Repeat("stream ", 200)
	h := Compress(CompressOptions{MinSize: 1})(Recover(RecoverOptions{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, first)
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Error(err)
			return
		}
		select {
		case <-finish:
		case <-r.Context().Done():
			return
		}
		_, _ = io.WriteString(w, "done")
	})))
	h = AccessLog(AccessLogOptions{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})(h)
	srv := httptest.NewServer(h)
	defer srv.Close()
	defer close(finish)
	client := srv.Client()
	client.Timeout = 5 * time.Second
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got := make([]byte, len(first))
	if _, err := io.ReadFull(resp.Body, got); err != nil || string(got) != first {
		t.Fatalf("stream unavailable before handler returned: %v", err)
	}
}

func TestCompressRejectsInvalidConfiguration(t *testing.T) {
	for _, opts := range []CompressOptions{{Level: -3}, {Level: 10}, {MinSize: -1}, {ContentTypes: []string{"text/*"}}, {ContentTypes: []string{"text/plain;broken"}}} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("invalid configuration accepted: %+v", opts)
				}
			}()
			Compress(opts)
		}()
	}
}
