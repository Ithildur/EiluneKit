package auth_test

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	authcore "github.com/Ithildur/EiluneKit/auth"
)

func TestMemoryLockoutCapacityPolicies(t *testing.T) {
	for _, policy := range []authcore.CapacityPolicy{authcore.AllowUntrackedKeys, authcore.RejectNewKeys} {
		t.Run(strconv.Itoa(int(policy)), func(t *testing.T) {
			now := time.Now()
			lockout := authcore.NewMemoryLockout(authcore.MemoryLockoutOptions{
				MaxKeys: 2, MaxFailures: 2, Window: time.Minute, Lockout: time.Hour,
				CapacityPolicy: policy, Now: func() time.Time { return now },
			})
			ctx := t.Context()
			for _, key := range []string{"locked", "locked", "counting"} {
				if _, _, err := lockout.RecordFailure(ctx, key); err != nil {
					t.Fatal(err)
				}
			}
			until := now.Add(time.Hour)
			if _, locked, err := lockout.Check(ctx, "new"); locked ||
				(policy == authcore.RejectNewKeys && !errors.Is(err, authcore.ErrLockoutCapacity)) ||
				(policy == authcore.AllowUntrackedKeys && err != nil) {
				t.Fatalf("capacity check: locked=%v err=%v", locked, err)
			}
			if _, _, err := lockout.RecordFailure(ctx, "new"); !errors.Is(err, authcore.ErrLockoutCapacity) {
				t.Fatalf("new record at capacity: %v", err)
			}
			if got, locked, err := lockout.RecordFailure(ctx, "counting"); err != nil || !locked || !got.Equal(until) {
				t.Fatalf("existing failure count lost: until=%v locked=%v err=%v", got, locked, err)
			}
			now = now.Add(2 * time.Minute)
			if _, _, err := lockout.RecordFailure(ctx, "new"); !errors.Is(err, authcore.ErrLockoutCapacity) {
				t.Fatalf("failure window expiry removed active locks: %v", err)
			}
			for _, key := range []string{"locked", "counting"} {
				if got, locked, err := lockout.Check(ctx, key); err != nil || !locked || !got.Equal(until) {
					t.Fatalf("lock %s changed: until=%v locked=%v err=%v", key, got, locked, err)
				}
			}
			now = until
			if _, locked, err := lockout.Check(ctx, "new"); err != nil || locked {
				t.Fatalf("expiry check: %v", err)
			}
			if _, _, err := lockout.RecordFailure(ctx, "new"); err != nil {
				t.Fatal(err)
			}
			if _, _, err := lockout.RecordFailure(ctx, "other"); err != nil {
				t.Fatal(err)
			}
			now = now.Add(time.Minute + time.Nanosecond)
			if _, _, err := lockout.RecordFailure(ctx, "after-window"); err != nil {
				t.Fatal(err)
			}
			if err := lockout.Clear(ctx, "after-window"); err != nil {
				t.Fatal(err)
			}
			if _, _, err := lockout.RecordFailure(ctx, "after-clear"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMemoryLockoutConcurrentAdmission(t *testing.T) {
	for _, policy := range []authcore.CapacityPolicy{authcore.AllowUntrackedKeys, authcore.RejectNewKeys} {
		t.Run(strconv.Itoa(int(policy)), func(t *testing.T) {
			const capacity, workers = 4, 32
			lockout := authcore.NewMemoryLockout(authcore.MemoryLockoutOptions{
				MaxKeys: capacity, MaxFailures: 1, CapacityPolicy: policy,
			})
			var ready, done sync.WaitGroup
			ready.Add(workers)
			start := make(chan struct{})
			results := make(chan error, workers)
			for i := range workers {
				done.Go(func() {
					key := strconv.Itoa(i)
					_, _, err := lockout.Check(t.Context(), key)
					ready.Done()
					<-start
					if err != nil {
						results <- err
						return
					}
					_, _, err = lockout.RecordFailure(t.Context(), key)
					results <- err
				})
			}
			ready.Wait()
			close(start)
			done.Wait()
			close(results)
			accepted := 0
			for err := range results {
				if err == nil {
					accepted++
				} else if !errors.Is(err, authcore.ErrLockoutCapacity) {
					t.Fatal(err)
				}
			}
			if accepted != capacity {
				t.Fatalf("admitted %d records, want %d", accepted, capacity)
			}
			lockedCount := 0
			for i := range workers {
				if _, locked, _ := lockout.Check(t.Context(), strconv.Itoa(i)); locked {
					lockedCount++
				}
			}
			if lockedCount != capacity {
				t.Fatalf("retained %d locks, want %d", lockedCount, capacity)
			}
		})
	}
}

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
