package gorm_test

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	kitgorm "github.com/Ithildur/EiluneKit/postgres/gorm"
)

func TestConnectHonorsCancellationDuringHandshake(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		db, err := kitgorm.Connect(ctx, kitgorm.Config{
			Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port,
			User: "test", Database: "test", SSLMode: "disable",
		})
		if db != nil {
			pool, poolErr := db.DB()
			if poolErr == nil {
				_ = pool.Close()
			}
		}
		finished <- err
	}()
	select {
	case conn := <-accepted:
		t.Cleanup(func() { _ = conn.Close() })
	case err := <-finished:
		t.Fatalf("connection failed before handshake: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("connection did not reach the listener")
	}
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("database initialization ignored cancellation")
	}
}
