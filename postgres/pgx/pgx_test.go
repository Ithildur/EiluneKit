package pgx_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/Ithildur/EiluneKit/postgres/pgx"
)

func TestConnectTimeoutLimitsHandshake(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, _ := listener.Accept()
		accepted <- conn
	}()
	pool, err := pgx.NewPool(t.Context(), pgx.Config{
		Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port,
		User: "app", Database: "db", SSLMode: "disable", ConnectTimeout: 50 * time.Millisecond,
	})
	t.Cleanup(func() {
		_ = listener.Close()
		if conn := <-accepted; conn != nil {
			_ = conn.Close()
		}
		if pool != nil {
			pool.Close()
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if err := pgx.Ping(ctx, pool); err == nil || ctx.Err() != nil {
		t.Fatalf("handshake was not stopped by ConnectTimeout: ping=%v context=%v", err, ctx.Err())
	}
}
