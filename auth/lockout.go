package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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

var (
	// ErrLockoutMissing reports a missing login lockout.
	// ErrLockoutMissing 表示缺少登录锁定器。
	ErrLockoutMissing = errors.New("login lockout is required")
	// ErrLockoutKeyRequired reports an empty login lockout key.
	// ErrLockoutKeyRequired 表示缺少登录锁定 key。
	ErrLockoutKeyRequired = errors.New("login lockout key is required")
	// ErrLoginLocked reports a locked login key.
	// ErrLoginLocked 表示登录 key 已锁定。
	ErrLoginLocked = errors.New("login locked")
)

// LockedError carries the lockout expiration for ErrLoginLocked.
// LockedError 携带 ErrLoginLocked 的锁定过期时间。
type LockedError struct {
	Until time.Time
}

func (e LockedError) Error() string {
	return ErrLoginLocked.Error()
}

// Is reports whether target is ErrLoginLocked.
// Is 返回 target 是否为 ErrLoginLocked。
func (e LockedError) Is(target error) bool {
	return target == ErrLoginLocked
}

// Lockout tracks failed login attempts for a caller-provided non-empty key.
// Empty keys should return ErrLockoutKeyRequired.
// Lockout 跟踪调用方提供的非空 key 的失败登录尝试。
// 空 key 应返回 ErrLockoutKeyRequired。
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
// It stores fixed-size hashes of caller-provided keys.
// Use NewMemoryLockout to create it; the zero value is not ready for use.
// MemoryLockout 在内存中跟踪登录失败。
// 它存储调用方提供 key 的固定长度 hash。
// 使用 NewMemoryLockout 创建；零值不可直接使用。
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
// Empty key returns ErrLockoutKeyRequired.
// Check 返回 key 当前是否已锁定。
// 空 key 返回 ErrLockoutKeyRequired。
func (l *MemoryLockout) Check(ctx context.Context, key string) (time.Time, bool, error) {
	contextutil.Require(ctx)
	if l == nil {
		return time.Time{}, false, ErrLockoutMissing
	}
	key, err := memoryLockoutKey(key)
	if err != nil {
		return time.Time{}, false, err
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
// Empty key returns ErrLockoutKeyRequired.
// RecordFailure 记录一次失败尝试并返回 key 是否被锁定。
// 空 key 返回 ErrLockoutKeyRequired。
func (l *MemoryLockout) RecordFailure(ctx context.Context, key string) (time.Time, bool, error) {
	contextutil.Require(ctx)
	if l == nil {
		return time.Time{}, false, ErrLockoutMissing
	}
	key, err := memoryLockoutKey(key)
	if err != nil {
		return time.Time{}, false, err
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
// Empty key returns ErrLockoutKeyRequired.
// Clear 从锁定状态中移除 key。
// 空 key 返回 ErrLockoutKeyRequired。
func (l *MemoryLockout) Clear(ctx context.Context, key string) error {
	contextutil.Require(ctx)
	if l == nil {
		return ErrLockoutMissing
	}
	key, err := memoryLockoutKey(key)
	if err != nil {
		return err
	}
	l.mu.Lock()
	delete(l.items, key)
	l.mu.Unlock()
	return nil
}

func memoryLockoutKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", ErrLockoutKeyRequired
	}
	sum := sha256.Sum256([]byte(key))
	return "sha256:" + hex.EncodeToString(sum[:]), nil
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
