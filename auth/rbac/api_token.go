package rbac

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	authcore "github.com/Ithildur/EiluneKit/auth"
	"github.com/Ithildur/EiluneKit/contextutil"

	"github.com/google/uuid"
)

const (
	defaultAPITokenBytes  = 32
	defaultAPITokenPrefix = 12
	apiTokenSecretPrefix  = "ekt_"
)

// APIToken is the stored metadata for an opaque bearer API token.
// Hash must be derived from the raw token; the raw token must not be stored.
// APIToken 是 opaque bearer API token 的存储元数据。
// Hash 必须由原始 token 派生；不得存储原始 token。
type APIToken struct {
	ID         string
	Name       string
	Hash       string
	Prefix     string
	CreatedBy  string
	Role       string
	Scopes     []string
	ExpiresAt  time.Time
	CreatedAt  time.Time
	LastUsedAt time.Time
	Disabled   bool
}

// Principal returns the authenticated principal for t.
// Principal 返回 t 对应的已认证主体。
func (t APIToken) Principal() authcore.Principal {
	return authcore.Principal{
		Subject: t.ID,
		Role:    t.Role,
		Scopes:  append([]string(nil), t.Scopes...),
		Kind:    authcore.PrincipalKindAPIToken,
	}
}

// APITokenStore persists API token metadata and usage.
// GetAPITokenByHash must not mutate token usage state.
// MarkAPITokenUsed is called only after Service accepts the token.
// APITokenStore 持久化 API token 元数据与使用状态。
// GetAPITokenByHash 不得修改 token 使用状态。
// MarkAPITokenUsed 只会在 Service 接受 token 后调用。
type APITokenStore interface {
	GetAPITokenByHash(ctx context.Context, hash string) (APIToken, bool, error)
	MarkAPITokenUsed(ctx context.Context, id string, at time.Time) error
	CreateAPIToken(ctx context.Context, token APIToken) error
	DeleteAPIToken(ctx context.Context, id string) (APIToken, bool, error)
}

// CreateAPITokenRequest describes a token to create.
// CreateAPITokenRequest 描述待创建的 token。
type CreateAPITokenRequest struct {
	ID        string
	Name      string
	CreatedBy string
	Role      string
	Scopes    []string
	ExpiresAt time.Time
}

// CreatedAPIToken returns public metadata and a one-time plaintext token.
// Token.Hash is empty; only APITokenStore receives the storage hash.
// CreatedAPIToken 返回公开元数据和仅展示一次的明文 token。
// Token.Hash 为空；只有 APITokenStore 会收到存储 hash。
type CreatedAPIToken struct {
	Token APIToken
	Raw   string
}

// HashToken returns the stable storage hash for a bearer token.
// HashToken 返回 bearer token 的稳定存储 hash。
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func createRawAPIToken() (string, string, string, error) {
	buf := make([]byte, defaultAPITokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", "", fmt.Errorf("generate api token: %w", err)
	}
	raw := apiTokenSecretPrefix + base64.RawURLEncoding.EncodeToString(buf)
	prefixLen := defaultAPITokenPrefix
	if len(raw) < prefixLen {
		prefixLen = len(raw)
	}
	prefix := raw[:prefixLen]
	return raw, prefix, HashToken(raw), nil
}

func normalizeAPITokenForStore(req CreateAPITokenRequest, raw, prefix, hash string, now time.Time) (APIToken, error) {
	id := strings.TrimSpace(req.ID)
	if id == "" {
		id = uuid.NewString()
	}
	createdBy := strings.TrimSpace(req.CreatedBy)
	if createdBy == "" {
		return APIToken{}, ErrUserIDRequired
	}
	token := APIToken{
		ID:        id,
		Name:      strings.TrimSpace(req.Name),
		Hash:      hash,
		Prefix:    prefix,
		CreatedBy: createdBy,
		Role:      strings.TrimSpace(req.Role),
		Scopes:    append([]string(nil), req.Scopes...),
		ExpiresAt: req.ExpiresAt,
		CreatedAt: now,
	}
	if raw == "" || prefix == "" || hash == "" {
		return APIToken{}, ErrBearerTokenRequired
	}
	return token, nil
}

func validAPIToken(token APIToken, now time.Time) bool {
	if strings.TrimSpace(token.ID) == "" || strings.TrimSpace(token.Hash) == "" {
		return false
	}
	if token.Disabled {
		return false
	}
	if !token.ExpiresAt.IsZero() && !token.ExpiresAt.After(now) {
		return false
	}
	return true
}

func publicAPIToken(token APIToken) APIToken {
	token.Hash = ""
	token.Scopes = append([]string(nil), token.Scopes...)
	return token
}

func requireAPITokenStore(ctx context.Context, store APITokenStore) (context.Context, error) {
	ctx = contextutil.Require(ctx)
	if store == nil {
		return ctx, ErrAPITokenStoreMissing
	}
	return ctx, nil
}
