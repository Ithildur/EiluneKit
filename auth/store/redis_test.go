package store

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestRedisStoreRevokeSessionMapsBackendErrors(t *testing.T) {
	client := redis.NewClient(&redis.Options{
		Addr: "127.0.0.1:6379",
		Dialer: func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New("dial failed")
		},
	})
	t.Cleanup(func() {
		_ = client.Close()
	})

	store := NewRedisStore(client, RedisOptions{
		WriteTimeout: 50 * time.Millisecond,
	})

	err := store.RevokeSession(context.Background(), "session-1")
	if !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("expected ErrStoreUnavailable, got %v", err)
	}
}
