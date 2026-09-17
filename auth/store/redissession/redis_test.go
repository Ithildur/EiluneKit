package redissession

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os/exec"
	"sync"
	"testing"
	"time"

	authjwt "github.com/Ithildur/EiluneKit/auth/jwt"
	authstore "github.com/Ithildur/EiluneKit/auth/store"
	kitredis "github.com/Ithildur/EiluneKit/redis"

	"github.com/redis/go-redis/v9"
)

type afterCommand func(redis.Cmder)

func (h afterCommand) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h afterCommand) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

func (h afterCommand) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		err := next(ctx, cmd)
		if err == nil {
			h(cmd)
		}
		return err
	}
}

func TestClearUserSessionsPreservesConcurrentCreation(t *testing.T) {
	for _, populated := range []bool{false, true} {
		name := "empty snapshot"
		if populated {
			name = "populated snapshot"
		}
		t.Run(name, func(t *testing.T) {
			client := newTestRedisClient(t)
			store := New(client, Options{})
			ctx := t.Context()
			state := authstore.SessionState{UserID: "user-1", RefreshID: "refresh", ExpiresAt: time.Now().Add(2 * time.Hour)}
			if populated {
				if err := store.CreateSession(ctx, "old", state); err != nil {
					t.Fatal(err)
				}
			}
			state.ExpiresAt = time.Now().Add(5 * time.Minute)
			create := sync.OnceFunc(func() {
				if err := store.CreateSession(ctx, "new", state); err != nil {
					t.Fatal(err)
				}
			})
			client.AddHook(afterCommand(func(cmd redis.Cmder) {
				if cmd.Name() == "zrange" {
					create()
				}
			}))
			if err := store.ClearUserSessions(ctx, state.UserID); err != nil {
				t.Fatal(err)
			}
			if _, exists, err := store.Session(ctx, "old"); err != nil || exists {
				t.Fatalf("old session: exists=%v error=%v", exists, err)
			}
			if _, exists, err := store.Session(ctx, "new"); err != nil || !exists {
				t.Fatalf("new session: exists=%v error=%v", exists, err)
			}
			// Check before Sessions can repair the index TTL.
			// 在 Sessions 有机会修复索引 TTL 之前检查。
			assertIndexTTL(t, client, store.userSessionsKey(state.UserID), state.ExpiresAt)
			sessions, err := store.Sessions(ctx, state.UserID)
			if err != nil || len(sessions) != 1 || sessions[0].ID != "new" {
				t.Fatalf("sessions=%v error=%v", sessions, err)
			}
			if err := store.ClearUserSessions(ctx, state.UserID); err != nil {
				t.Fatal(err)
			}
			if count, err := client.Exists(ctx, store.sessionKey("new"), store.userSessionsKey(state.UserID)).Result(); err != nil || count != 0 {
				t.Fatalf("cleanup left keys: count=%d error=%v", count, err)
			}
		})
	}
}

func TestClearAllSessionsPreservesConcurrentSessionsAndNamespaces(t *testing.T) {
	client := newTestRedisClient(t)
	for _, prefixes := range [][2]string{
		{"test:[ab]:", "test:a:"}, {"test:*:", "test:other:"},
		{"test:?:", "test:x:"}, {`test:\x:`, "test:x:"},
	} {
		t.Run(prefixes[0], func(t *testing.T) {
			ctx := t.Context()
			own := New(client, Options{Prefix: prefixes[0]})
			other := New(client, Options{Prefix: prefixes[1]})
			state := authstore.SessionState{UserID: "user-1", RefreshID: "refresh", ExpiresAt: time.Now().Add(time.Hour)}
			if err := other.CreateSession(ctx, "session", state); err != nil {
				t.Fatal(err)
			}
			if _, err := own.BumpUserVersion(ctx, state.UserID); err != nil {
				t.Fatal(err)
			}
			manager, err := authjwt.New("0123456789abcdef0123456789abcdef", own)
			if err != nil {
				t.Fatal(err)
			}
			access, _, refresh, _, err := manager.IssueSessionTokens(ctx, state.UserID, authjwt.IssueOptions{})
			if err != nil {
				t.Fatal(err)
			}
			var concurrentRefresh string
			var concurrentExpiry time.Time
			create := sync.OnceFunc(func() {
				_, _, concurrentRefresh, concurrentExpiry, err = manager.IssueSessionTokens(ctx, state.UserID, authjwt.IssueOptions{})
				if err != nil {
					t.Fatal(err)
				}
			})
			client.AddHook(afterCommand(func(cmd redis.Cmder) {
				if scan, ok := cmd.(*redis.ScanCmd); ok {
					if _, cursor := scan.Val(); cursor == 0 {
						// Add after the final scan selected records, before cleanup finishes.
						// 在最后一次扫描选定记录后、清理完成前新增会话。
						create()
					}
				}
			}))
			if err := manager.ClearAllSessions(ctx); err != nil {
				t.Fatal(err)
			}
			if _, valid, err := manager.ValidateAccessToken(ctx, access); err != nil || valid {
				t.Fatalf("old access token: valid=%v error=%v", valid, err)
			}
			if _, valid, err := manager.ValidateRefreshToken(ctx, refresh); err != nil || valid {
				t.Fatalf("old refresh token: valid=%v error=%v", valid, err)
			}
			claims, valid, err := manager.ValidateRefreshToken(ctx, concurrentRefresh)
			if err != nil || !valid {
				t.Fatalf("concurrent refresh token: valid=%v error=%v", valid, err)
			}
			key := own.userSessionsKey(state.UserID)
			assertIndexTTL(t, client, key, concurrentExpiry)
			sessions, err := manager.Sessions(ctx, state.UserID)
			if err != nil || len(sessions) != 1 || sessions[0].ID != claims.SessionID {
				t.Fatalf("sessions=%v error=%v", sessions, err)
			}
			if count, err := client.ZCard(ctx, key).Result(); err != nil || count != 1 {
				t.Fatalf("stale index members remain: count=%d error=%v", count, err)
			}
			if count, err := client.Exists(ctx, other.sessionKey("session"), other.userSessionsKey(state.UserID)).Result(); err != nil || count != 2 {
				t.Fatalf("other namespace changed: count=%d error=%v", count, err)
			}
			if version, err := own.UserVersion(ctx, state.UserID); err != nil || version != 1 {
				t.Fatalf("user version changed: version=%d error=%v", version, err)
			}
		})
	}
}

func TestStoreDeadlinesWithKitClient(t *testing.T) {
	client := newTestRedisClient(t)
	store := New(client, Options{ReadTimeout: 20 * time.Millisecond, WriteTimeout: 20 * time.Millisecond})
	for _, operation := range []struct {
		name string
		run  func(context.Context, string) (int64, error)
	}{
		{"read", store.UserVersion}, {"write", store.BumpUserVersion},
	} {
		t.Run(operation.name, func(t *testing.T) {
			ctx := t.Context()
			if err := client.Do(ctx, "CLIENT", "PAUSE", 250, "ALL").Err(); err != nil {
				t.Fatal(err)
			}
			if _, err := operation.run(ctx, "user-1"); !errors.Is(err, authstore.ErrStoreUnavailable) {
				t.Fatalf("operation exceeded its deadline without failing: %v", err)
			}
			if err := client.Ping(ctx).Err(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

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

func TestStoreSessionIndexLifecycle(t *testing.T) {
	client := newTestRedisClient(t)
	ctx := t.Context()
	store := New(client, Options{Prefix: "test:" + t.Name() + ":"})
	userID := "user-1"
	key := store.userSessionsKey(userID)
	now := time.Now()
	shortExp := now.Add(5 * time.Minute)
	longExp := now.Add(2 * time.Hour)
	extendedExp := now.Add(3 * time.Hour)

	if err := client.ZAdd(ctx, key, redis.Z{
		Score: float64(now.Add(-time.Hour).Unix()), Member: "sid-expired",
	}).Err(); err != nil {
		t.Fatalf("seed expired index member: %v", err)
	}
	for _, session := range []struct {
		id      string
		expires time.Time
	}{
		{"sid-long", longExp}, {"sid-short", shortExp},
	} {
		if err := store.CreateSession(ctx, session.id, authstore.SessionState{
			UserID: userID, RefreshID: "refresh-old", ExpiresAt: session.expires,
		}); err != nil {
			t.Fatalf("create %s: %v", session.id, err)
		}
	}
	if err := client.ZScore(ctx, key, "sid-expired").Err(); !errors.Is(err, redis.Nil) {
		t.Fatalf("expected expired index member to be pruned, got %v", err)
	}
	assertIndexTTL(t, client, key, longExp)

	if rotated, err := store.RotateRefresh(ctx, "sid-long", userID, 0, "refresh-old", "refresh-new", extendedExp); err != nil || !rotated {
		t.Fatalf("rotate refresh: rotated=%v error=%v", rotated, err)
	}
	assertIndexTTL(t, client, key, extendedExp)

	if err := store.RevokeSession(ctx, "sid-long"); err != nil {
		t.Fatalf("revoke long session: %v", err)
	}
	if err := client.ZScore(ctx, key, "sid-long").Err(); !errors.Is(err, redis.Nil) {
		t.Fatalf("expected revoked session to be removed from index, got %v", err)
	}
	assertIndexTTL(t, client, key, shortExp)

	if err := client.ZAdd(ctx, key, redis.Z{
		Score: float64(longExp.Unix()), Member: "sid-stale",
	}).Err(); err != nil {
		t.Fatalf("seed stale member: %v", err)
	}
	if err := client.PExpire(ctx, key, time.Until(longExp)+sessionIndexTTLGrace).Err(); err != nil {
		t.Fatalf("seed stale index TTL: %v", err)
	}
	sessions, err := store.Sessions(ctx, userID)
	if err != nil || len(sessions) != 1 || sessions[0].ID != "sid-short" {
		t.Fatalf("sessions=%v error=%v", sessions, err)
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

	client, err := kitredis.NewClient(kitredis.Config{
		Addr: "127.0.0.1:" + port,
	})
	t.Cleanup(func() {
		_ = client.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	})
	if err != nil {
		t.Fatal(err)
	}

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
