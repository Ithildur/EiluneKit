package redissession

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os/exec"
	"testing"
	"time"

	authstore "github.com/Ithildur/EiluneKit/auth/store"

	"github.com/redis/go-redis/v9"
)

func TestStoreRevokeSessionMapsBackendErrors(t *testing.T) {
	client := redis.NewClient(&redis.Options{
		Addr: "127.0.0.1:6379",
		Dialer: func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New("dial failed")
		},
	})
	t.Cleanup(func() {
		_ = client.Close()
	})

	store := New(client, Options{
		WriteTimeout: 50 * time.Millisecond,
	})

	err := store.RevokeSession(context.Background(), "session-1")
	if !errors.Is(err, authstore.ErrStoreUnavailable) {
		t.Fatalf("expected authstore.ErrStoreUnavailable, got %v", err)
	}
}

func TestStoreSessionIndexTTLTracksLatestSession(t *testing.T) {
	client := newTestRedisClient(t)
	ctx := context.Background()
	store := New(client, Options{
		Prefix: "test:" + t.Name() + ":",
	})
	userID := "user-1"
	key := store.userSessionsKey(userID)
	now := time.Now().UTC()
	longExp := now.Add(2 * time.Hour)
	shortExp := now.Add(5 * time.Minute)

	if err := client.ZAdd(ctx, key, redis.Z{
		Score:  float64(now.Add(-time.Hour).Unix()),
		Member: "sid-expired",
	}).Err(); err != nil {
		t.Fatalf("seed expired index member: %v", err)
	}
	if err := store.CreateSession(ctx, "sid-long", authstore.SessionState{
		UserID:    userID,
		RefreshID: "refresh-long",
		ExpiresAt: longExp,
	}); err != nil {
		t.Fatalf("create long session: %v", err)
	}
	if err := store.CreateSession(ctx, "sid-short", authstore.SessionState{
		UserID:    userID,
		RefreshID: "refresh-short",
		ExpiresAt: shortExp,
	}); err != nil {
		t.Fatalf("create short session: %v", err)
	}

	if err := client.ZScore(ctx, key, "sid-expired").Err(); !errors.Is(err, redis.Nil) {
		t.Fatalf("expected expired index member to be pruned, got %v", err)
	}
	assertIndexTTL(t, client, key, longExp)
}

func TestStoreRotateRefreshExtendsSessionIndexTTL(t *testing.T) {
	client := newTestRedisClient(t)
	ctx := context.Background()
	store := New(client, Options{
		Prefix: "test:" + t.Name() + ":",
	})
	userID := "user-1"
	now := time.Now().UTC()
	shortExp := now.Add(5 * time.Minute)
	longExp := now.Add(2 * time.Hour)

	if err := store.CreateSession(ctx, "sid-1", authstore.SessionState{
		UserID:    userID,
		RefreshID: "refresh-old",
		ExpiresAt: shortExp,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	rotated, err := store.RotateRefresh(ctx, "sid-1", userID, 0, "refresh-old", "refresh-new", longExp)
	if err != nil {
		t.Fatalf("rotate refresh: %v", err)
	}
	if !rotated {
		t.Fatal("expected refresh rotation to succeed")
	}

	assertIndexTTL(t, client, store.userSessionsKey(userID), longExp)
}

func TestStoreRevokeSessionUpdatesSessionIndexTTL(t *testing.T) {
	client := newTestRedisClient(t)
	ctx := context.Background()
	store := New(client, Options{
		Prefix: "test:" + t.Name() + ":",
	})
	userID := "user-1"
	now := time.Now().UTC()
	shortExp := now.Add(5 * time.Minute)
	longExp := now.Add(2 * time.Hour)

	if err := store.CreateSession(ctx, "sid-short", authstore.SessionState{
		UserID:    userID,
		RefreshID: "refresh-short",
		ExpiresAt: shortExp,
	}); err != nil {
		t.Fatalf("create short session: %v", err)
	}
	if err := store.CreateSession(ctx, "sid-long", authstore.SessionState{
		UserID:    userID,
		RefreshID: "refresh-long",
		ExpiresAt: longExp,
	}); err != nil {
		t.Fatalf("create long session: %v", err)
	}
	if err := store.RevokeSession(ctx, "sid-long"); err != nil {
		t.Fatalf("revoke long session: %v", err)
	}

	key := store.userSessionsKey(userID)
	if err := client.ZScore(ctx, key, "sid-long").Err(); !errors.Is(err, redis.Nil) {
		t.Fatalf("expected revoked session to be removed from index, got %v", err)
	}
	assertIndexTTL(t, client, key, shortExp)
}

func TestStoreSessionsUpdatesSessionIndexTTLAfterRemovingStaleMember(t *testing.T) {
	client := newTestRedisClient(t)
	ctx := context.Background()
	store := New(client, Options{
		Prefix: "test:" + t.Name() + ":",
	})
	userID := "user-1"
	now := time.Now().UTC()
	shortExp := now.Add(5 * time.Minute)
	longExp := now.Add(2 * time.Hour)

	if err := store.CreateSession(ctx, "sid-short", authstore.SessionState{
		UserID:    userID,
		RefreshID: "refresh-short",
		ExpiresAt: shortExp,
	}); err != nil {
		t.Fatalf("create short session: %v", err)
	}
	key := store.userSessionsKey(userID)
	if err := client.ZAdd(ctx, key, redis.Z{
		Score:  float64(longExp.Unix()),
		Member: "sid-stale",
	}).Err(); err != nil {
		t.Fatalf("seed stale member: %v", err)
	}
	if err := client.PExpire(ctx, key, time.Until(longExp)+sessionIndexTTLGrace).Err(); err != nil {
		t.Fatalf("seed stale index TTL: %v", err)
	}

	sessions, err := store.Sessions(ctx, userID)
	if err != nil {
		t.Fatalf("sessions: %v", err)
	}
	if got, want := len(sessions), 1; got != want {
		t.Fatalf("expected %d session, got %#v", want, sessions)
	}
	if sessions[0].ID != "sid-short" {
		t.Fatalf("unexpected session: %#v", sessions[0])
	}
	if err := client.ZScore(ctx, key, "sid-stale").Err(); !errors.Is(err, redis.Nil) {
		t.Fatalf("expected stale session to be removed from index, got %v", err)
	}
	assertIndexTTL(t, client, key, shortExp)
}

func assertIndexTTL(t *testing.T, client *redis.Client, key string, exp time.Time) {
	t.Helper()
	ttl, err := client.PTTL(context.Background(), key).Result()
	if err != nil {
		t.Fatalf("read index TTL: %v", err)
	}
	if ttl <= 0 {
		t.Fatalf("expected index TTL, got %s", ttl)
	}
	minTTL := time.Until(exp) + sessionIndexTTLGrace - 10*time.Second
	maxTTL := time.Until(exp) + sessionIndexTTLGrace + 10*time.Second
	if ttl < minTTL || ttl > maxTTL {
		t.Fatalf("expected index TTL between %s and %s, got %s", minTTL, maxTTL, ttl)
	}
}

func newTestRedisClient(t *testing.T) *redis.Client {
	t.Helper()

	bin, err := exec.LookPath("redis-server")
	if err != nil {
		t.Skip("redis-server not found")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on test Redis port: %v", err)
	}
	addr := listener.Addr().String()
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("parse test Redis address: %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("close test Redis listener: %v", err)
	}

	var stderr bytes.Buffer
	cmd := exec.Command(
		bin,
		"--bind", "127.0.0.1",
		"--port", port,
		"--save", "",
		"--appendonly", "no",
		"--dir", t.TempDir(),
		"--loglevel", "warning",
	)
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start redis-server: %v: %s", err, stderr.String())
	}

	client := redis.NewClient(&redis.Options{
		Addr: "127.0.0.1:" + port,
	})
	t.Cleanup(func() {
		_ = client.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := client.Ping(ctx).Err(); err == nil {
			return client
		}
		select {
		case <-ctx.Done():
			t.Fatalf("wait for redis-server: %v: %s", ctx.Err(), stderr.String())
		case <-ticker.C:
		}
	}
}
