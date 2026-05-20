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

	//lint:ignore SA1012 nil context is the panic contract under test
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

	//lint:ignore SA1012 nil parent context is the panic contract under test
	_, _ = WithTimeout(nil, time.Second, func(ctx context.Context) (struct{}, error) {
		t.Fatal("callback should not be called")
		return struct{}{}, nil
	})
}
