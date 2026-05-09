package redis

import (
	"crypto/tls"
	"testing"
)

func TestNewClientKeepsTLSConfig(t *testing.T) {
	tlsConfig := &tls.Config{
		ServerName: "redis.example.com",
	}

	client, err := NewClient(Config{
		Addr:      "redis.example.com:6379",
		TLSConfig: tlsConfig,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close()
	})

	if got := client.Options().TLSConfig; got != tlsConfig {
		t.Fatalf("expected TLSConfig to be preserved")
	}
}
