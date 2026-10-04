package gorm_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"testing"
	"time"

	kitgorm "github.com/Ithildur/EiluneKit/postgres/gorm"

	"github.com/jackc/pgx/v5/pgproto3"
	gormcore "gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestConnectLoggerParameterPolicy(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(fmt.Sprintf("explicit=%v", explicit), func(t *testing.T) {
			var out bytes.Buffer
			base := logger.New(log.New(&out, "", 0), logger.Config{LogLevel: logger.Warn})
			previous := logger.Default
			logger.Default = base
			t.Cleanup(func() { logger.Default = previous })
			cfg := postgresHandshake(t)
			if explicit {
				cfg.Logger = base
			}
			db, err := kitgorm.Connect(t.Context(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			pool, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = pool.Close() })
			if err := db.Callback().Raw().Before("gorm:raw").Register("test:query_error", func(tx *gormcore.DB) {
				tx.AddError(errors.New("query failed"))
			}); err != nil {
				t.Fatal(err)
			}
			for _, queryDB := range []*gormcore.DB{db, db.Debug()} {
				out.Reset()
				const secret = "secret-query-parameter"
				err := queryDB.Session(&gormcore.Session{DryRun: true}).Exec("SELECT ?", secret).Error
				if err == nil {
					t.Fatal("expected query error")
				}
				output := out.String()
				if !strings.Contains(output, "SELECT") || !strings.Contains(output, "query failed") {
					t.Fatalf("lost query diagnostics: %q", output)
				}
				if strings.Contains(output, secret) != explicit {
					t.Fatalf("incorrect parameter policy: %q", output)
				}
			}
		})
	}
}

func postgresHandshake(t *testing.T) kitgorm.Config {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	t.Cleanup(func() {
		_ = listener.Close()
		if err := <-done; err != nil {
			t.Errorf("postgres handshake: %v", err)
		}
	})
	go func() {
		done <- func() error {
			conn, err := listener.Accept()
			if err != nil {
				return err
			}
			defer conn.Close()
			if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
				return err
			}
			backend := pgproto3.NewBackend(conn, conn)
			if _, err := backend.ReceiveStartupMessage(); err != nil {
				return err
			}
			backend.Send(&pgproto3.AuthenticationOk{})
			backend.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
			if err := backend.Flush(); err != nil {
				return err
			}
			message, err := backend.Receive()
			if err != nil {
				return err
			}
			if query, ok := message.(*pgproto3.Query); !ok || query.String != "-- ping" {
				return fmt.Errorf("expected ping, got %T", message)
			}
			backend.Send(&pgproto3.EmptyQueryResponse{})
			backend.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
			return backend.Flush()
		}()
	}()
	return kitgorm.Config{
		Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port,
		User: "test", Database: "test", SSLMode: "disable",
	}
}

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
