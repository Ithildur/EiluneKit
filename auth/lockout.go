package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"maps"
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
	// ErrLockoutCapacity reports that no new failure record can be stored.
	// ErrLockoutCapacity 表示无法保存新的失败记录。
	ErrLockoutCapacity = errors.New("login lockout capacity exhausted")
)

// CapacityPolicy controls checks for unknown keys when the lockout table is full.
// CapacityPolicy 控制锁定表满时如何检查未知 key。
type CapacityPolicy uint8

const (
	// AllowUntrackedKeys permits checks even when new failures cannot be recorded.
	// AllowUntrackedKeys 在无法记录新失败时仍允许检查通过。
	AllowUntrackedKeys CapacityPolicy = iota
	// RejectNewKeys rejects checks for unknown keys when no capacity is available.
	// RejectNewKeys 在没有容量时拒绝检查未知 key。
	RejectNewKeys
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
// RecordFailure must report an active lock without resetting or extending it.
// Lockout 跟踪调用方提供的非空 key 的失败登录尝试。
// 空 key 应返回 ErrLockoutKeyRequired。
// RecordFailure 必须返回已有且未到期的锁定，不得重置或延长它。
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
	// CapacityPolicy defaults to AllowUntrackedKeys. Unexpired records are never evicted.
	// CapacityPolicy 默认使用 AllowUntrackedKeys。未过期记录不会被淘汰。
	CapacityPolicy CapacityPolicy
	Now            func() time.Time
}

// MemoryLockout tracks login failures in memory.
// It stores fixed-size hashes of caller-provided keys.
// Use NewMemoryLockout to create it; the zero value is not ready for use.
// MemoryLockout 在内存中跟踪登录失败。
// 它存储调用方提供 key 的固定长度 hash。
// 使用 NewMemoryLockout 创建；零值不可直接使用。
type MemoryLockout struct {
	mu    sync.Mutex
	items map[string]lockoutItem
	opts  MemoryLockoutOptions
	now   func() time.Time
}

type lockoutItem struct {
	failures int
	first    time.Time
	locked   time.Time
}

// NewMemoryLockout returns an in-memory Lockout.
// An unsupported CapacityPolicy panics.
// NewMemoryLockout 返回内存版 Lockout。
// 不支持的 CapacityPolicy 会引发 panic。
func NewMemoryLockout(opts MemoryLockoutOptions) *MemoryLockout {
	if opts.CapacityPolicy != AllowUntrackedKeys && opts.CapacityPolicy != RejectNewKeys {
		panic("auth: invalid lockout capacity policy")
	}
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
		items: make(map[string]lockoutItem),
		opts:  opts,
		now:   now,
	}
}

// Check reports whether key is currently locked.
// Empty key returns ErrLockoutKeyRequired.
// RejectNewKeys returns ErrLockoutCapacity for unknown keys when full.
// Check 返回 key 当前是否已锁定。
// 空 key 返回 ErrLockoutKeyRequired。
// RejectNewKeys 在容量满时对未知 key 返回 ErrLockoutCapacity。
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
	if ok {
		if !l.expired(item, now) {
			return item.locked, item.locked.After(now), nil
		}
		delete(l.items, key)
	}
	if l.opts.CapacityPolicy == RejectNewKeys && !l.hasCapacity(now) {
		return time.Time{}, false, ErrLockoutCapacity
	}
	return time.Time{}, false, nil
}

// RecordFailure records a failed attempt and reports whether it locked key.
// An active lock keeps its original expiration regardless of the failure window.
// Empty key returns ErrLockoutKeyRequired.
// Both capacity policies return ErrLockoutCapacity when a new record cannot fit after expiration cleanup.
// RecordFailure 记录一次失败尝试并返回 key 是否被锁定。
// 已生效的锁定保持原过期时间，不受失败统计窗口影响。
// 空 key 返回 ErrLockoutKeyRequired。
// 两种容量策略均在清理过期记录后仍无法保存新记录时返回 ErrLockoutCapacity。
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
	item, exists := l.items[key]
	if item.locked.After(now) {
		return item.locked, true, nil
	}
	if !exists && !l.hasCapacity(now) {
		return time.Time{}, false, ErrLockoutCapacity
	}
	if !exists || l.expired(item, now) {
		item = lockoutItem{first: now}
	}
	item.failures++
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

func (l *MemoryLockout) hasCapacity(now time.Time) bool {
	if len(l.items) < l.opts.MaxKeys {
		return true
	}
	maps.DeleteFunc(l.items, func(_ string, item lockoutItem) bool {
		return l.expired(item, now)
	})
	return len(l.items) < l.opts.MaxKeys
}

func (l *MemoryLockout) expired(item lockoutItem, now time.Time) bool {
	if !item.locked.IsZero() {
		return !item.locked.After(now)
	}
	return now.Sub(item.first) > l.opts.Window
}
