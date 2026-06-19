package auth

import (
	"context"
	"strings"
	"testing"
)

func TestMemoryLockoutStoresBoundedKey(t *testing.T) {
	lockout := NewMemoryLockout(MemoryLockoutOptions{})
	rawKey := "ip:192.0.2.10|username:" + strings.Repeat("a", 64*1024)

	if _, _, err := lockout.RecordFailure(context.Background(), rawKey); err != nil {
		t.Fatalf("record failure: %v", err)
	}
	if len(lockout.items) != 1 {
		t.Fatalf("expected one lockout item, got %d", len(lockout.items))
	}
	for stored := range lockout.items {
		if len(stored) != len("sha256:")+64 {
			t.Fatalf("stored key length = %d, want %d", len(stored), len("sha256:")+64)
		}
		if strings.Contains(stored, strings.Repeat("a", 128)) {
			t.Fatal("stored key retained attacker-controlled payload")
		}
	}
	if err := lockout.Clear(context.Background(), rawKey); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if len(lockout.items) != 0 {
		t.Fatalf("expected clear to remove hashed item, got %d items", len(lockout.items))
	}
}
