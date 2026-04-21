// Package jwt provides JWT issuance and validation backed by user/session state.
// Package jwt 提供基于用户/会话状态的 JWT 签发与校验。
package jwt

import (
	"context"
	"errors"
	"strings"
	"time"

	authstore "github.com/Ithildur/EiluneKit/auth/store"
	"github.com/Ithildur/EiluneKit/contextutil"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	defaultAccessTTL  = 15 * time.Minute
	defaultRefreshTTL = 14 * 24 * time.Hour
	defaultIssuer     = "kit"
	defaultAudience   = "client"
)

const (
	TokenKindAccess  = "access"
	TokenKindRefresh = "refresh"
)

var (
	// ErrUnauthorized reports an invalid or expired token.
	// ErrUnauthorized 表示 token 无效或已过期。
	ErrUnauthorized = errors.New("unauthorized")
	// ErrManagerMisconfigured reports missing Manager state.
	// ErrManagerMisconfigured 表示 Manager 缺少必要状态。
	ErrManagerMisconfigured = errors.New("jwt manager is misconfigured")
	// ErrUserIDRequired reports a missing user ID.
	// ErrUserIDRequired 表示缺少 user ID。
	ErrUserIDRequired = errors.New("user id is required")
	// ErrSessionIDRequired reports a missing session ID.
	// ErrSessionIDRequired 表示缺少 session ID。
	ErrSessionIDRequired = errors.New("session id is required")
	// ErrStoreUnavailable reports store unavailability.
	// ErrStoreUnavailable 表示 store 不可用。
	ErrStoreUnavailable = errors.New("token store unavailable")
)

type claimsContextKey struct{}

// IssueOptions controls token issuance behavior.
// IssueOptions 控制 token 签发行为。
type IssueOptions struct {
	SessionOnly bool `json:"session_only,omitempty"`
}

// Claims contains token claims used by the auth flow.
// Claims 保存认证流程使用的 token claims。
type Claims struct {
	Kind      string `json:"kind"`
	SessionID string `json:"sid"`
	Version   int64  `json:"ver"`
	jwt.RegisteredClaims
}

// RefreshResult contains refreshed tokens.
// RefreshResult 保存刷新后的 token。
type RefreshResult struct {
	Access           string    `json:"access,omitempty"`
	AccessExpiresAt  time.Time `json:"access_expires_at,omitempty"`
	Refresh          string    `json:"refresh,omitempty"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at,omitempty"`
	SessionOnly      bool      `json:"session_only,omitempty"`
}

// WithClaims stores claims on ctx.
// Use WithClaims(ctx, claims) before passing ctx downstream.
// WithClaims 将 claims 写入 ctx。
// 在向下游传递 ctx 前调用 WithClaims(ctx, claims)。
func WithClaims(ctx context.Context, claims Claims) context.Context {
	return context.WithValue(contextutil.Require(ctx), claimsContextKey{}, claims)
}

// ClaimsFromContext returns claims stored by WithClaims.
// ClaimsFromContext 返回 WithClaims 存入的 claims。
func ClaimsFromContext(ctx context.Context) (Claims, bool) {
	if ctx == nil {
		return Claims{}, false
	}
	claims, ok := ctx.Value(claimsContextKey{}).(Claims)
	return claims, ok
}

// Manager issues and validates JWTs backed by SessionStore.
// Manager 负责签发和校验由 SessionStore 支撑的 JWT。
type Manager struct {
	signingKey string
	store      authstore.SessionStore
	accessTTL  time.Duration
	refreshTTL time.Duration
	issuer     string
	audience   string
}

// ManagerOptions configures NewWithOptions.
// ManagerOptions 配置 NewWithOptions。
type ManagerOptions struct {
	Issuer     string
	Audience   string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

// DefaultManagerOptions returns default options.
// DefaultManagerOptions 返回默认选项。
func DefaultManagerOptions() ManagerOptions {
	return ManagerOptions{
		Issuer:     defaultIssuer,
		Audience:   defaultAudience,
		AccessTTL:  defaultAccessTTL,
		RefreshTTL: defaultRefreshTTL,
	}
}

// New returns a Manager with DefaultManagerOptions.
// Call New(signingKey, store).
// New 返回使用 DefaultManagerOptions 的 Manager。
// 调用 New(signingKey, store)。
func New(signingKey string, store authstore.SessionStore) (*Manager, error) {
	return NewWithOptions(signingKey, store, DefaultManagerOptions())
}

// NewWithOptions returns a Manager with explicit options.
// Use separate Manager values for different issuers, audiences, or TTLs.
// NewWithOptions 返回使用显式选项的 Manager。
// 不同 issuer、audience 或 TTL 应使用独立的 Manager。
func NewWithOptions(signingKey string, store authstore.SessionStore, opts ManagerOptions) (*Manager, error) {
	signingKey = strings.TrimSpace(signingKey)
	if signingKey == "" {
		return nil, errors.New("jwt signing key is empty")
	}
	if len(signingKey) < 32 {
		return nil, errors.New("jwt signing key must be at least 32 characters")
	}
	if store == nil {
		return nil, errors.New("jwt session store is required")
	}
	if strings.TrimSpace(opts.Issuer) == "" {
		opts.Issuer = defaultIssuer
	}
	if strings.TrimSpace(opts.Audience) == "" {
		opts.Audience = defaultAudience
	}
	if opts.AccessTTL <= 0 {
		opts.AccessTTL = defaultAccessTTL
	}
	if opts.RefreshTTL <= 0 {
		opts.RefreshTTL = defaultRefreshTTL
	}
	return &Manager{
		signingKey: signingKey,
		store:      store,
		accessTTL:  opts.AccessTTL,
		refreshTTL: opts.RefreshTTL,
		issuer:     opts.Issuer,
		audience:   opts.Audience,
	}, nil
}

// IssueSessionTokens issues access and refresh tokens.
// IssueSessionTokens 签发 access 与 refresh token。
func (m *Manager) IssueSessionTokens(ctx context.Context, userID string, opts IssueOptions) (access string, accessExp time.Time, refresh string, refreshExp time.Time, err error) {
	if err := m.requireConfigured(); err != nil {
		return "", time.Time{}, "", time.Time{}, err
	}
	ctx = contextutil.Require(ctx)
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", time.Time{}, "", time.Time{}, ErrUserIDRequired
	}
	version, err := m.userVersion(ctx, userID)
	if err != nil {
		return "", time.Time{}, "", time.Time{}, err
	}
	sessionID := uuid.NewString()
	access, accessExp, _, err = m.signToken(userID, TokenKindAccess, sessionID, version, m.accessTTL)
	if err != nil {
		return "", time.Time{}, "", time.Time{}, err
	}
	refresh, refreshExp, refreshID, err := m.signToken(userID, TokenKindRefresh, sessionID, version, m.refreshTTL)
	if err != nil {
		return "", time.Time{}, "", time.Time{}, err
	}
	if err := m.createSession(ctx, sessionID, authstore.SessionState{
		UserID:      userID,
		RefreshID:   refreshID,
		ExpiresAt:   refreshExp,
		SessionOnly: opts.SessionOnly,
	}); err != nil {
		return "", time.Time{}, "", time.Time{}, err
	}
	return access, accessExp, refresh, refreshExp, nil
}

// ValidateAccessToken validates an access token.
// ok reports whether the token was accepted.
// ValidateAccessToken 校验 access token。
// ok 表示 token 是否通过校验。
func (m *Manager) ValidateAccessToken(ctx context.Context, tokenStr string) (Claims, bool, error) {
	if err := m.requireConfigured(); err != nil {
		return Claims{}, false, err
	}
	if strings.TrimSpace(tokenStr) == "" {
		return Claims{}, false, nil
	}
	claims, _, ok, err := m.validateTokenWithClaims(ctx, TokenKindAccess, tokenStr)
	return claims, ok, err
}

// ValidateRefreshToken validates a refresh token.
// ok reports whether the token was accepted.
// ValidateRefreshToken 校验 refresh token。
// ok 表示 token 是否通过校验。
func (m *Manager) ValidateRefreshToken(ctx context.Context, tokenStr string) (Claims, bool, error) {
	if err := m.requireConfigured(); err != nil {
		return Claims{}, false, err
	}
	if strings.TrimSpace(tokenStr) == "" {
		return Claims{}, false, nil
	}
	claims, _, ok, err := m.validateTokenWithClaims(ctx, TokenKindRefresh, tokenStr)
	return claims, ok, err
}

// RotateRefreshTokens rotates a refresh token.
// ok reports whether the old refresh token was accepted.
// RotateRefreshTokens 轮换 refresh token。
// ok 表示旧 refresh token 是否通过校验。
func (m *Manager) RotateRefreshTokens(ctx context.Context, oldRefresh string) (RefreshResult, bool, error) {
	if err := m.requireConfigured(); err != nil {
		return RefreshResult{}, false, err
	}
	if strings.TrimSpace(oldRefresh) == "" {
		return RefreshResult{}, false, nil
	}

	claims, session, ok, err := m.validateTokenWithClaims(ctx, TokenKindRefresh, oldRefresh)
	if err != nil {
		return RefreshResult{}, false, err
	}
	if !ok {
		return RefreshResult{}, false, nil
	}

	access, accessExp, _, err := m.signToken(claims.Subject, TokenKindAccess, claims.SessionID, claims.Version, m.accessTTL)
	if err != nil {
		return RefreshResult{}, false, err
	}
	newRefresh, newRefreshExp, newRefreshID, err := m.signToken(claims.Subject, TokenKindRefresh, claims.SessionID, claims.Version, m.refreshTTL)
	if err != nil {
		return RefreshResult{}, false, err
	}

	rotated, err := m.rotateRefresh(ctx, claims.SessionID, claims.Subject, claims.Version, claims.ID, newRefreshID, newRefreshExp)
	if err != nil {
		return RefreshResult{}, false, err
	}
	if !rotated {
		return RefreshResult{}, false, nil
	}

	return RefreshResult{
		Access:           access,
		AccessExpiresAt:  accessExp,
		Refresh:          newRefresh,
		RefreshExpiresAt: newRefreshExp,
		SessionOnly:      session.SessionOnly,
	}, true, nil
}

// RevokeAccess revokes the session bound to tokenStr.
// RevokeAccess 吊销 tokenStr 绑定的 session。
func (m *Manager) RevokeAccess(ctx context.Context, tokenStr string) error {
	return m.revokeSessionFromToken(ctx, TokenKindAccess, tokenStr)
}

// RevokeRefresh revokes the session bound to tokenStr.
// RevokeRefresh 吊销 tokenStr 绑定的 session。
func (m *Manager) RevokeRefresh(ctx context.Context, tokenStr string) error {
	return m.revokeSessionFromToken(ctx, TokenKindRefresh, tokenStr)
}

// RevokeSession revokes one session.
// ok reports whether the session belonged to userID.
// RevokeSession 吊销一个 session。
// ok 表示该 session 是否属于 userID。
func (m *Manager) RevokeSession(ctx context.Context, userID, sessionID string) (bool, error) {
	if err := m.requireConfigured(); err != nil {
		return false, err
	}
	ctx = contextutil.Require(ctx)
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return false, ErrUserIDRequired
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false, ErrSessionIDRequired
	}
	session, ok, err := m.session(ctx, sessionID)
	if err != nil {
		return false, err
	}
	if !ok || session.UserID != userID {
		return false, nil
	}
	if err := m.revokeSession(ctx, sessionID); err != nil {
		return false, err
	}
	return true, nil
}

// RevokeAllSessions revokes all sessions for userID.
// RevokeAllSessions 吊销 userID 的全部 session。
func (m *Manager) RevokeAllSessions(ctx context.Context, userID string) error {
	if err := m.requireConfigured(); err != nil {
		return err
	}
	ctx = contextutil.Require(ctx)
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return ErrUserIDRequired
	}
	if _, err := m.bumpUserVersion(ctx, userID); err != nil {
		return err
	}
	return nil
}

func (m *Manager) signToken(userID, kind, sessionID string, version int64, ttl time.Duration) (signed string, exp time.Time, jti string, err error) {
	if err := m.requireConfigured(); err != nil {
		return "", time.Time{}, "", err
	}
	now := time.Now().UTC()
	exp = now.Add(ttl)
	jti = uuid.NewString()

	claims := Claims{
		Kind:      kind,
		SessionID: sessionID,
		Version:   version,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			Subject:   userID,
			Issuer:    m.issuer,
			Audience:  jwt.ClaimStrings{m.audience},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err = token.SignedString([]byte(m.signingKey))
	if err != nil {
		return "", time.Time{}, "", err
	}
	return signed, exp, jti, nil
}

func (m *Manager) validateTokenWithClaims(ctx context.Context, expectedKind, tokenStr string) (Claims, authstore.SessionState, bool, error) {
	if err := m.requireConfigured(); err != nil {
		return Claims{}, authstore.SessionState{}, false, err
	}
	ctx = contextutil.Require(ctx)
	claims, ok := m.parseToken(tokenStr)
	if !ok {
		return Claims{}, authstore.SessionState{}, false, nil
	}
	if claims.Kind != expectedKind {
		return Claims{}, authstore.SessionState{}, false, nil
	}
	if strings.TrimSpace(claims.Subject) == "" || strings.TrimSpace(claims.SessionID) == "" {
		return Claims{}, authstore.SessionState{}, false, nil
	}
	if claims.Version < 0 {
		return Claims{}, authstore.SessionState{}, false, nil
	}
	session, ok, err := m.validateClaims(ctx, claims, expectedKind == TokenKindRefresh)
	if err != nil {
		return Claims{}, authstore.SessionState{}, false, err
	}
	return claims, session, ok, nil
}

func (m *Manager) validateClaims(ctx context.Context, claims Claims, requireRefreshMatch bool) (authstore.SessionState, bool, error) {
	version, err := m.userVersion(ctx, claims.Subject)
	if err != nil {
		return authstore.SessionState{}, false, err
	}
	if version != claims.Version {
		return authstore.SessionState{}, false, nil
	}
	session, ok, err := m.session(ctx, claims.SessionID)
	if err != nil {
		return authstore.SessionState{}, false, err
	}
	if !ok {
		return authstore.SessionState{}, false, nil
	}
	if session.UserID != claims.Subject {
		return authstore.SessionState{}, false, nil
	}
	if requireRefreshMatch && session.RefreshID != claims.ID {
		return authstore.SessionState{}, false, nil
	}
	if !session.ExpiresAt.IsZero() && !session.ExpiresAt.After(time.Now().UTC()) {
		return authstore.SessionState{}, false, nil
	}
	return session, true, nil
}

func (m *Manager) revokeSessionFromToken(ctx context.Context, expectedKind, tokenStr string) error {
	if err := m.requireConfigured(); err != nil {
		return err
	}
	ctx = contextutil.Require(ctx)
	claims, ok := m.parseToken(tokenStr)
	if !ok || claims.Kind != expectedKind {
		return ErrUnauthorized
	}
	return m.revokeSession(ctx, claims.SessionID)
}

func (m *Manager) parseToken(tokenStr string) (Claims, bool) {
	if strings.TrimSpace(tokenStr) == "" {
		return Claims{}, false
	}
	if m == nil {
		return Claims{}, false
	}

	claims := Claims{}
	parser := jwt.NewParser(m.parserOptions()...)
	token, err := parser.ParseWithClaims(tokenStr, &claims, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, jwt.ErrTokenUnverifiable
		}
		return []byte(m.signingKey), nil
	})
	if err != nil || token == nil || !token.Valid {
		return Claims{}, false
	}
	if claims.ID == "" || claims.Subject == "" || claims.Kind == "" || claims.SessionID == "" {
		return Claims{}, false
	}
	return claims, true
}

func (m *Manager) parserOptions() []jwt.ParserOption {
	opts := []jwt.ParserOption{
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
	}
	if strings.TrimSpace(m.issuer) != "" {
		opts = append(opts, jwt.WithIssuer(m.issuer))
	}
	if strings.TrimSpace(m.audience) != "" {
		opts = append(opts, jwt.WithAudience(m.audience))
	}
	return opts
}

func (m *Manager) userVersion(ctx context.Context, userID string) (int64, error) {
	if err := m.requireConfigured(); err != nil {
		return 0, err
	}
	version, err := m.store.UserVersion(ctx, userID)
	if err != nil {
		if errors.Is(err, authstore.ErrStoreUnavailable) {
			return 0, ErrStoreUnavailable
		}
		return 0, err
	}
	return version, nil
}

func (m *Manager) bumpUserVersion(ctx context.Context, userID string) (int64, error) {
	if err := m.requireConfigured(); err != nil {
		return 0, err
	}
	version, err := m.store.BumpUserVersion(ctx, userID)
	if err != nil {
		if errors.Is(err, authstore.ErrStoreUnavailable) {
			return 0, ErrStoreUnavailable
		}
		return 0, err
	}
	return version, nil
}

func (m *Manager) createSession(ctx context.Context, sessionID string, state authstore.SessionState) error {
	if err := m.requireConfigured(); err != nil {
		return err
	}
	if err := m.store.CreateSession(ctx, sessionID, state); err != nil {
		if errors.Is(err, authstore.ErrStoreUnavailable) {
			return ErrStoreUnavailable
		}
		return err
	}
	return nil
}

func (m *Manager) session(ctx context.Context, sessionID string) (authstore.SessionState, bool, error) {
	if err := m.requireConfigured(); err != nil {
		return authstore.SessionState{}, false, err
	}
	state, ok, err := m.store.Session(ctx, sessionID)
	if err != nil {
		if errors.Is(err, authstore.ErrStoreUnavailable) {
			return authstore.SessionState{}, false, ErrStoreUnavailable
		}
		return authstore.SessionState{}, false, err
	}
	return state, ok, nil
}

func (m *Manager) rotateRefresh(ctx context.Context, sessionID, userID string, expectedVersion int64, oldRefreshID, newRefreshID string, exp time.Time) (bool, error) {
	if err := m.requireConfigured(); err != nil {
		return false, err
	}
	rotated, err := m.store.RotateRefresh(ctx, sessionID, userID, expectedVersion, oldRefreshID, newRefreshID, exp)
	if err != nil {
		if errors.Is(err, authstore.ErrStoreUnavailable) {
			return false, ErrStoreUnavailable
		}
		return false, err
	}
	return rotated, nil
}

func (m *Manager) revokeSession(ctx context.Context, sessionID string) error {
	if err := m.requireConfigured(); err != nil {
		return err
	}
	if err := m.store.RevokeSession(ctx, sessionID); err != nil {
		if errors.Is(err, authstore.ErrStoreUnavailable) {
			return ErrStoreUnavailable
		}
		return err
	}
	return nil
}

func (m *Manager) requireConfigured() error {
	switch {
	case m == nil:
		return ErrManagerMisconfigured
	case strings.TrimSpace(m.signingKey) == "":
		return ErrManagerMisconfigured
	case m.store == nil:
		return ErrManagerMisconfigured
	default:
		return nil
	}
}
