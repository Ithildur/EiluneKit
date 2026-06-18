package rbac

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/Ithildur/EiluneKit/contextutil"
)

const (
	defaultLockoutFailures = 5
	defaultLockoutWindow   = 15 * time.Minute
	defaultLockoutDuration = 15 * time.Minute
	defaultLockoutMaxKeys  = 10000
)

// Lockout tracks failed login attempts for a caller-provided key.
// Lockout 跟踪调用方提供的 key 的失败登录尝试。
type Lockout interface {
	Check(ctx context.Context, key string) (until time.Time, locked bool, err error)
	RecordFailure(ctx context.Context, key string) (until time.Time, locked bool, err error)
	Clear(ctx context.Context, key string) error
}

// MemoryLockoutOptions configures NewMemoryLockout.
// MemoryLockoutOptions 配置 NewMemoryLockout。
type MemoryLockoutOptions struct {
	MaxFailures int
	Window      time.Duration
	Lockout     time.Duration
	MaxKeys     int
	Now         func() time.Time
}

// MemoryLockout tracks login failures in memory.
// MemoryLockout 在内存中跟踪登录失败。
type MemoryLockout struct {
	mu     sync.Mutex
	items  map[string]lockoutItem
	opts   MemoryLockoutOptions
	now    func() time.Time
	maxKey int
}

type lockoutItem struct {
	failures int
	first    time.Time
	locked   time.Time
	updated  time.Time
}

// NewMemoryLockout returns an in-memory Lockout.
// NewMemoryLockout 返回内存版 Lockout。
func NewMemoryLockout(opts MemoryLockoutOptions) *MemoryLockout {
	if opts.MaxFailures <= 0 {
		opts.MaxFailures = defaultLockoutFailures
	}
	if opts.Window <= 0 {
		opts.Window = defaultLockoutWindow
	}
	if opts.Lockout <= 0 {
		opts.Lockout = defaultLockoutDuration
	}
	if opts.MaxKeys <= 0 {
		opts.MaxKeys = defaultLockoutMaxKeys
	}
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &MemoryLockout{
		items:  make(map[string]lockoutItem),
		opts:   opts,
		now:    now,
		maxKey: opts.MaxKeys,
	}
}

// Check reports whether key is currently locked.
// Check 返回 key 当前是否已锁定。
func (l *MemoryLockout) Check(ctx context.Context, key string) (time.Time, bool, error) {
	contextutil.Require(ctx)
	if l == nil {
		return time.Time{}, false, nil
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return time.Time{}, false, nil
	}
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	item, ok := l.items[key]
	if !ok {
		return time.Time{}, false, nil
	}
	if !item.locked.IsZero() {
		if item.locked.After(now) {
			return item.locked, true, nil
		}
		delete(l.items, key)
		return time.Time{}, false, nil
	}
	if now.Sub(item.first) > l.opts.Window {
		delete(l.items, key)
	}
	return time.Time{}, false, nil
}

// RecordFailure records a failed attempt and reports whether it locked key.
// RecordFailure 记录一次失败尝试并返回 key 是否被锁定。
func (l *MemoryLockout) RecordFailure(ctx context.Context, key string) (time.Time, bool, error) {
	contextutil.Require(ctx)
	if l == nil {
		return time.Time{}, false, nil
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return time.Time{}, false, nil
	}
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ensureCapacity(now, key)
	item := l.items[key]
	if item.first.IsZero() || now.Sub(item.first) > l.opts.Window || (!item.locked.IsZero() && !item.locked.After(now)) {
		item = lockoutItem{first: now}
	}
	item.failures++
	item.updated = now
	if item.failures >= l.opts.MaxFailures {
		item.locked = now.Add(l.opts.Lockout)
	}
	l.items[key] = item
	return item.locked, !item.locked.IsZero() && item.locked.After(now), nil
}

// Clear removes key from the lockout state.
// Clear 从锁定状态中移除 key。
func (l *MemoryLockout) Clear(ctx context.Context, key string) error {
	contextutil.Require(ctx)
	if l == nil {
		return nil
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	l.mu.Lock()
	delete(l.items, key)
	l.mu.Unlock()
	return nil
}

func (l *MemoryLockout) ensureCapacity(now time.Time, keep string) {
	if l.maxKey <= 0 || len(l.items) < l.maxKey {
		return
	}
	for key, item := range l.items {
		if key == keep {
			continue
		}
		if (!item.locked.IsZero() && !item.locked.After(now)) || (item.locked.IsZero() && now.Sub(item.first) > l.opts.Window) {
			delete(l.items, key)
		}
	}
	if len(l.items) < l.maxKey {
		return
	}
	var oldestKey string
	var oldest time.Time
	for key, item := range l.items {
		if key == keep {
			continue
		}
		if oldestKey == "" || item.updated.Before(oldest) {
			oldestKey = key
			oldest = item.updated
		}
	}
	if oldestKey != "" {
		delete(l.items, oldestKey)
	}
}
