package auth_test

import (
	"context"
	"errors"
	"testing"

	authcore "github.com/Ithildur/EiluneKit/auth"
)

func TestMemoryLockoutRequiresKey(t *testing.T) {
	lockout := authcore.NewMemoryLockout(authcore.MemoryLockoutOptions{})

	if _, _, err := lockout.Check(context.Background(), " "); !errors.Is(err, authcore.ErrLockoutKeyRequired) {
		t.Fatalf("expected ErrLockoutKeyRequired from Check, got %v", err)
	}
	if _, _, err := lockout.RecordFailure(context.Background(), " "); !errors.Is(err, authcore.ErrLockoutKeyRequired) {
		t.Fatalf("expected ErrLockoutKeyRequired from RecordFailure, got %v", err)
	}
	if err := lockout.Clear(context.Background(), " "); !errors.Is(err, authcore.ErrLockoutKeyRequired) {
		t.Fatalf("expected ErrLockoutKeyRequired from Clear, got %v", err)
	}
}

func TestMemoryLockoutRejectsNilReceiver(t *testing.T) {
	var lockout *authcore.MemoryLockout

	if _, _, err := lockout.Check(context.Background(), "ip:127.0.0.1"); !errors.Is(err, authcore.ErrLockoutMissing) {
		t.Fatalf("expected ErrLockoutMissing from Check, got %v", err)
	}
	if _, _, err := lockout.RecordFailure(context.Background(), "ip:127.0.0.1"); !errors.Is(err, authcore.ErrLockoutMissing) {
		t.Fatalf("expected ErrLockoutMissing from RecordFailure, got %v", err)
	}
	if err := lockout.Clear(context.Background(), "ip:127.0.0.1"); !errors.Is(err, authcore.ErrLockoutMissing) {
		t.Fatalf("expected ErrLockoutMissing from Clear, got %v", err)
	}
}
