package store

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMemoryStoreRotateRefreshReplacesOldRefreshID(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	exp := time.Now().UTC().Add(time.Hour)

	if err := s.CreateSession(ctx, "sid-1", SessionState{
		UserID:      "user-1",
		RefreshID:   "old-refresh",
		ExpiresAt:   exp,
		SessionOnly: true,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	rotated, err := s.RotateRefresh(ctx, "sid-1", "user-1", 0, "old-refresh", "new-refresh", exp)
	if err != nil {
		t.Fatalf("rotate refresh: %v", err)
	}
	if !rotated {
		t.Fatalf("expected refresh rotation to succeed")
	}

	state, ok, err := s.Session(ctx, "sid-1")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if !ok {
		t.Fatalf("expected session to remain active")
	}
	if state.RefreshID != "new-refresh" {
		t.Fatalf("expected refresh id new-refresh, got %q", state.RefreshID)
	}
	if !state.SessionOnly {
		t.Fatal("expected session_only to be preserved")
	}
}

func TestMemoryStoreRotateRefreshConcurrentSingleSuccess(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	exp := time.Now().UTC().Add(time.Hour)

	if err := s.CreateSession(ctx, "sid-1", SessionState{
		UserID:    "user-1",
		RefreshID: "old-refresh",
		ExpiresAt: exp,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	const workers = 32
	start := make(chan struct{})
	var wg sync.WaitGroup
	var successCount int32

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			rotated, err := s.RotateRefresh(ctx, "sid-1", "user-1", 0, "old-refresh", "new-refresh", exp)
			if err != nil {
				t.Errorf("rotate refresh worker %d: %v", i, err)
				return
			}
			if rotated {
				atomic.AddInt32(&successCount, 1)
			}
		}(i)
	}

	close(start)
	wg.Wait()

	if got := atomic.LoadInt32(&successCount); got != 1 {
		t.Fatalf("expected exactly 1 successful refresh rotation, got %d", got)
	}
}

func TestMemoryStoreBumpUserVersionInvalidatesExpectedVersion(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	exp := time.Now().UTC().Add(time.Hour)

	if _, err := s.BumpUserVersion(ctx, "user-1"); err != nil {
		t.Fatalf("bump version: %v", err)
	}
	if err := s.CreateSession(ctx, "sid-1", SessionState{
		UserID:    "user-1",
		RefreshID: "refresh-1",
		ExpiresAt: exp,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	rotated, err := s.RotateRefresh(ctx, "sid-1", "user-1", 0, "refresh-1", "refresh-2", exp)
	if err != nil {
		t.Fatalf("rotate refresh: %v", err)
	}
	if rotated {
		t.Fatalf("expected rotate to fail when user version mismatches")
	}
}
