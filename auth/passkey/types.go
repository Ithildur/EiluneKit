// Package passkey provides WebAuthn registration and authentication.
// Package passkey 提供 WebAuthn 注册与身份验证。
package passkey

import (
	"context"
	"errors"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

var (
	// ErrRejected reports an invalid, expired, or already consumed ceremony.
	// ErrRejected 表示流程无效、过期或已消费。
	ErrRejected = errors.New("passkey authentication rejected")
	// ErrNotFound reports a credential absent from the requested user's account.
	// ErrNotFound 表示指定用户账户中不存在该凭据。
	ErrNotFound = errors.New("passkey not found")
	// ErrConflict reports an existing credential ID or flow ID.
	// ErrConflict 表示凭据 ID 或流程 ID 已存在。
	ErrConflict = errors.New("passkey already exists")
	// ErrInvalidName reports an empty or oversized credential name.
	// ErrInvalidName 表示凭据名称为空或过长。
	ErrInvalidName = errors.New("passkey name must contain 1 to 100 characters")
	// ErrCapacity reports a full flow store.
	// ErrCapacity 表示流程存储已满。
	ErrCapacity = errors.New("passkey flow capacity reached")
)

// User contains current account information. ID must be immutable, opaque,
// non-empty, without surrounding whitespace, and never reassigned.
// User 包含当前账户信息。ID 必须不变、不含个人信息、非空、无首尾空白且永不重新分配。
type User struct {
	ID          string
	Name        string
	DisplayName string
}

// Options configures one relying party. LookupUser must return ok=false for
// missing or disabled accounts. Stores must be scoped to this relying party.
// Options 配置单个依赖方。LookupUser 对不存在或禁用账户必须返回 ok=false。
// 存储必须限定在此依赖方范围内。
type Options struct {
	RPID          string
	RPDisplayName string
	Origins       []string
	Store         Store
	Flows         FlowStore
	LookupUser    func(context.Context, string) (User, bool, error)
	// Timeout defaults to five minutes.
	// Timeout 默认五分钟。
	Timeout time.Duration
}

// Credential is the persisted record. Data must be stored in full.
// Credential 是持久化记录。Data 必须完整保存。
type Credential struct {
	UserID     string
	Name       string
	CreatedAt  time.Time
	LastUsedAt time.Time
	Data       webauthn.Credential
}

// Store persists credentials. All methods must be concurrency-safe and return
// owned data. String credential IDs are unpadded base64url encodings of Data.ID.
// Insert must return ErrConflict for duplicate Data.ID and never overwrite a record.
// UpdateUsage must only update usage fields on an existing (id,userID) record;
// it must not insert, replace the key/owner/name, or decrease stored counters.
// Rename and Delete must also match both id and userID. False means not found.
// Store 持久化凭据。所有方法必须并发安全，返回独立数据。
// 字符串凭据 ID 是 Data.ID 的无填充 base64url 编码。
// Insert 遇到重复 Data.ID 必须返回 ErrConflict，不能覆盖记录。
// UpdateUsage 只能更新现有 (id,userID) 记录的使用状态，不能插入、替换密钥、所有者、名称或降低计数器。
// Rename 和 Delete 也必须匹配 id 和 userID；false 表示不存在。
type Store interface {
	Insert(context.Context, Credential) error
	Get(ctx context.Context, id string) (Credential, bool, error)
	List(ctx context.Context, userID string) ([]Credential, error)
	UpdateUsage(ctx context.Context, id, userID string, authenticator webauthn.Authenticator, flags webauthn.CredentialFlags, at time.Time) (bool, error)
	Rename(ctx context.Context, id, userID, name string) (bool, error)
	Delete(ctx context.Context, id, userID string) (bool, error)
}

// Flow is server-owned ceremony state; never accept it from clients.
// Flow 是服务端管理的流程状态，不能从客户端接收。
type Flow struct {
	RPID        string
	Operation   string
	UserID      string
	SessionOnly bool
	Session     webauthn.SessionData
}

// FlowStore stores ceremonies until Flow.Session.Expires. Create must not overwrite; Consume
// must atomically remove and return a flow, returning false for absent/expired IDs.
// FlowStore 存储流程至 Flow.Session.Expires。Create 不能覆盖；Consume 必须原子移除并返回流程，
// 对不存在或过期的 ID 返回 false。
type FlowStore interface {
	Create(ctx context.Context, id string, flow Flow) error
	Consume(ctx context.Context, id string) (Flow, bool, error)
}

// Registration contains browser creation options and an opaque flow ID.
// Registration 包含浏览器创建选项与不透明流程 ID。
type Registration struct {
	FlowID  string                       `json:"flow_id"`
	Options *protocol.CredentialCreation `json:"options"`
}

// Login contains browser assertion options and an opaque flow ID.
// Login 包含浏览器断言选项与不透明流程 ID。
type Login struct {
	FlowID  string                        `json:"flow_id"`
	Options *protocol.CredentialAssertion `json:"options"`
}

// LoginResult contains the verified account and server-bound session preference.
// LoginResult 包含已验证账户与绑定在服务端的会话偏好。
type LoginResult struct {
	User        User
	SessionOnly bool
}

// Info is user-visible credential metadata; it excludes the public key.
// Info 是用户可见的凭据元数据，不含公钥。
type Info struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt time.Time `json:"last_used_at,omitzero"`
}
