package contextutil

import (
	"context"
	"testing"
	"time"
)

func TestRequirePanicsOnNil(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for nil context")
		}
		if got, want := r, nilContextMessage; got != want {
			t.Fatalf("expected panic %q, got %v", want, got)
		}
	}()

	Require(nil)
}

func TestWithTimeoutPanicsOnNilParent(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for nil parent context")
		}
		if got, want := r, nilContextMessage; got != want {
			t.Fatalf("expected panic %q, got %v", want, got)
		}
	}()

	_, _ = WithTimeout(nil, time.Second, func(ctx context.Context) (struct{}, error) {
		t.Fatal("callback should not be called")
		return struct{}{}, nil
	})
}
