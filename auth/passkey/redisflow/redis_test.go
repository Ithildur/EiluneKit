package redisflow_test

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ithildur/EiluneKit/auth/passkey"
	"github.com/Ithildur/EiluneKit/auth/passkey/redisflow"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/redis/go-redis/v9"
)

func TestRedisFlowLifecycle(t *testing.T) {
	bin, err := exec.LookPath("redis-server")
	if err != nil {
		t.Skip("redis-server not found")
	}
	socket := filepath.Join(t.TempDir(), "redis.sock")
	cmd := exec.Command(bin, "--port", "0", "--unixsocket", socket, "--unixsocketperm", "700", "--save", "", "--appendonly", "no")
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(&redis.Options{Network: "unix", Addr: socket, ContextTimeoutEnabled: true})
	t.Cleanup(func() { _ = client.Close(); _ = cmd.Process.Kill(); _ = cmd.Wait() })
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for client.Ping(ctx).Err() != nil {
		select {
		case <-ctx.Done():
			t.Fatal("Redis startup timed out")
		case <-ticker.C:
		}
	}
	s, err := redisflow.New(client, "test:passkey:")
	if err != nil {
		t.Fatal(err)
	}
	flow := passkey.Flow{RPID: "example.com", Operation: "login", SessionOnly: true, Session: webauthn.SessionData{Challenge: "challenge", Expires: time.Now().Add(time.Minute)}}
	if err = s.Create(ctx, "once", flow); err != nil {
		t.Fatal(err)
	}
	if err = s.Create(ctx, "once", flow); !errors.Is(err, passkey.ErrConflict) {
		t.Fatal(err)
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			result, ok, err := s.Consume(ctx, "once")
			if err != nil {
				t.Error(err)
			}
			if ok {
				successes.Add(1)
				if !result.SessionOnly || result.Session.Challenge != "challenge" {
					t.Error("flow did not round-trip")
				}
			}
		})
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("consumed %d times", successes.Load())
	}
	flow.Session.Expires = time.Now().Add(20 * time.Millisecond)
	if err = s.Create(ctx, "expire", flow); err != nil {
		t.Fatal(err)
	}
	for time.Now().Before(flow.Session.Expires) {
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-ticker.C:
		}
	}
	if _, ok, err := s.Consume(ctx, "expire"); err != nil || ok {
		t.Fatalf("expired flow: %v %v", ok, err)
	}
	if err = client.Close(); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := s.Consume(ctx, "missing"); err == nil || ok {
		t.Fatal("Redis outage concealed as missing flow")
	}
}
