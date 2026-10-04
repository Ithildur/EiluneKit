package rbac

import authcore "github.com/Ithildur/EiluneKit/auth"

// Lockout tracks failed login attempts for a caller-provided non-empty key.
// Empty keys should return ErrLockoutKeyRequired.
// Lockout 跟踪调用方提供的非空 key 的失败登录尝试。
// 空 key 应返回 ErrLockoutKeyRequired。
type Lockout = authcore.Lockout

// MemoryLockoutOptions configures NewMemoryLockout.
// MemoryLockoutOptions 配置 NewMemoryLockout。
type MemoryLockoutOptions = authcore.MemoryLockoutOptions

// CapacityPolicy controls checks for unknown keys when the lockout table is full.
// CapacityPolicy 控制锁定表满时如何检查未知 key。
type CapacityPolicy = authcore.CapacityPolicy

const (
	// AllowUntrackedKeys permits checks even when new failures cannot be recorded.
	// AllowUntrackedKeys 在无法记录新失败时仍允许检查通过。
	AllowUntrackedKeys = authcore.AllowUntrackedKeys
	// RejectNewKeys rejects checks for unknown keys when no capacity is available.
	// RejectNewKeys 在没有容量时拒绝检查未知 key。
	RejectNewKeys = authcore.RejectNewKeys
)

// MemoryLockout tracks login failures in memory.
// Use NewMemoryLockout to create it; the zero value is not ready for use.
// MemoryLockout 在内存中跟踪登录失败。
// 使用 NewMemoryLockout 创建；零值不可直接使用。
type MemoryLockout = authcore.MemoryLockout

// NewMemoryLockout returns an in-memory Lockout.
// NewMemoryLockout 返回内存版 Lockout。
func NewMemoryLockout(opts MemoryLockoutOptions) *MemoryLockout {
	return authcore.NewMemoryLockout(opts)
}
