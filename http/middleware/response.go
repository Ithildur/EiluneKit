package middleware

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"

	"github.com/felixge/httpsnoop"
)

type responseState struct {
	status int
	conn   net.Conn
}

func (s *responseState) start(code int) {
	if s.status == 0 && (code == http.StatusSwitchingProtocols || code >= 200) {
		s.status = code
	}
}

func (s *responseState) wrap(w http.ResponseWriter) http.ResponseWriter {
	var writer http.ResponseWriter
	writer = httpsnoop.Wrap(w, httpsnoop.Hooks{
		WriteHeader: func(next httpsnoop.WriteHeaderFunc) httpsnoop.WriteHeaderFunc {
			return func(code int) {
				next(code)
				s.start(code)
			}
		},
		Write: func(next httpsnoop.WriteFunc) httpsnoop.WriteFunc {
			return func(b []byte) (int, error) {
				s.start(http.StatusOK)
				return next(b)
			}
		},
		Flush: func(next httpsnoop.FlushFunc) httpsnoop.FlushFunc {
			return func() {
				s.start(http.StatusOK)
				next()
			}
		},
		FlushError: func(next httpsnoop.FlushErrorFunc) httpsnoop.FlushErrorFunc {
			return func() error {
				status := s.status
				s.start(http.StatusOK)
				err := next()
				if errors.Is(err, http.ErrNotSupported) {
					s.status = status
				}
				return err
			}
		},
		ReadFrom: func(next httpsnoop.ReadFromFunc) httpsnoop.ReadFromFunc {
			return func(src io.Reader) (int64, error) {
				if s.status != 0 {
					return next(src)
				}
				// Observe actual writes; an empty copy does not start a response.
				// 观察实际写入；空复制不会开始响应。
				return io.Copy(struct{ io.Writer }{writer}, src)
			}
		},
		Hijack: func(next httpsnoop.HijackFunc) httpsnoop.HijackFunc {
			return func() (net.Conn, *bufio.ReadWriter, error) {
				conn, rw, err := next()
				if err == nil {
					s.conn = conn
				}
				return conn, rw, err
			}
		},
	})
	return writer
}
