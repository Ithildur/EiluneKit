package rbac

import (
	"context"
	"fmt"
	"strings"
	"time"

	authcore "github.com/Ithildur/EiluneKit/auth"
	authjwt "github.com/Ithildur/EiluneKit/auth/jwt"
	"github.com/Ithildur/EiluneKit/contextutil"
)

const eventRevokeTimeout = 5 * time.Second

// Service runs RBAC authentication flows without transport code.
// Service 执行与传输层无关的 RBAC 认证流程。
type Service struct {
	users     UserStore
	passwords PasswordVerifier
	tokens    TokenManager
	apiTokens APITokenStore
	lockout   Lockout
	events    Events
	now       func() time.Time
}

// ServiceOptions configures NewService.
// ServiceOptions 配置 NewService。
type ServiceOptions struct {
	Users     UserStore
	Passwords PasswordVerifier
	Tokens    TokenManager
	APITokens APITokenStore
	Lockout   Lockout
	Events    Events
	Now       func() time.Time
}

// NewService returns a Service.
// APITokens is optional. Lockout defaults to NewMemoryLockout.
// Login requires Users, Passwords, and Tokens.
// NewService 返回 Service。
// APITokens 可选。Lockout 默认使用 NewMemoryLockout。
// 登录需要 Users、Passwords 和 Tokens。
func NewService(opts ServiceOptions) (*Service, error) {
	if opts.Users == nil {
		return nil, ErrUserStoreMissing
	}
	if opts.Passwords == nil {
		return nil, ErrPasswordVerifierMissing
	}
	if opts.Tokens == nil {
		return nil, ErrTokenManagerMissing
	}
	lockout := opts.Lockout
	if lockout == nil {
		lockout = NewMemoryLockout(MemoryLockoutOptions{})
	}
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{
		users:     opts.Users,
		passwords: opts.Passwords,
		tokens:    opts.Tokens,
		apiTokens: opts.APITokens,
		lockout:   lockout,
		events:    opts.Events,
		now:       now,
	}, nil
}

// Login verifies credentials and issues access and refresh tokens.
// ok reports whether credentials were accepted.
// Login 校验凭据并签发 access 与 refresh token。
// ok 表示凭据是否通过校验。
func (s *Service) Login(ctx context.Context, req LoginRequest) (Tokens, bool, error) {
	ctx = contextutil.Require(ctx)
	if err := s.requireLogin(); err != nil {
		return Tokens{}, false, err
	}
	req.Username = strings.TrimSpace(req.Username)
	req.LockoutKey = strings.TrimSpace(req.LockoutKey)
	if s.lockout != nil && req.LockoutKey == "" {
		return Tokens{}, false, ErrLockoutKeyRequired
	}
	if req.Username == "" || req.Password == "" {
		if err := s.rejectLogin(ctx, req, "", LoginFailureInvalidCredentials); err != nil {
			return Tokens{}, false, err
		}
		return Tokens{}, false, nil
	}
	if until, locked, err := s.locked(ctx, req.LockoutKey); err != nil {
		return Tokens{}, false, err
	} else if locked {
		if err := s.emitLoginFailure(ctx, LoginFailure{
			Username: req.Username,
			Reason:   LoginFailureLocked,
			At:       s.now(),
		}); err != nil {
			return Tokens{}, false, err
		}
		return Tokens{}, false, LockedError{Until: until}
	}

	user, ok, err := s.users.GetUserByUsername(ctx, req.Username)
	if err != nil {
		return Tokens{}, false, fmt.Errorf("load user by username: %w", err)
	}
	if !ok {
		if err := s.rejectLogin(ctx, req, "", LoginFailureInvalidCredentials); err != nil {
			return Tokens{}, false, err
		}
		return Tokens{}, false, nil
	}
	if strings.TrimSpace(user.ID) == "" {
		return Tokens{}, false, ErrUserIDRequired
	}
	if user.Disabled {
		if err := s.rejectLogin(ctx, req, user.ID, LoginFailureDisabled); err != nil {
			return Tokens{}, false, err
		}
		return Tokens{}, false, nil
	}
	passwordOK, err := s.passwords.VerifyPassword(ctx, user, req.Password)
	if err != nil {
		return Tokens{}, false, fmt.Errorf("verify password: %w", err)
	}
	if !passwordOK {
		if err := s.rejectLogin(ctx, req, user.ID, LoginFailureInvalidCredentials); err != nil {
			return Tokens{}, false, err
		}
		return Tokens{}, false, nil
	}
	if err := s.clearLockout(ctx, req.LockoutKey); err != nil {
		return Tokens{}, false, err
	}

	access, accessExp, refresh, refreshExp, err := s.tokens.IssueSessionTokens(ctx, user.ID, IssueOptions{
		SessionOnly: req.SessionOnly,
	})
	if err != nil {
		return Tokens{}, false, fmt.Errorf("issue session tokens: %w", err)
	}
	principal := user.Principal()
	if err := s.events.loginSuccess(ctx, principal); err != nil {
		revokeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), eventRevokeTimeout)
		defer cancel()
		if revokeErr := s.tokens.RevokeRefresh(revokeCtx, refresh); revokeErr != nil {
			return Tokens{}, false, fmt.Errorf("%w: login success hook failed: %w; revoke refresh: %w", ErrEventFailed, err, revokeErr)
		}
		return Tokens{}, false, fmt.Errorf("%w: login success hook failed: %w", ErrEventFailed, err)
	}
	return Tokens{
		Principal:        principal,
		AccessToken:      access,
		AccessExpiresAt:  accessExp,
		RefreshToken:     refresh,
		RefreshExpiresAt: refreshExp,
		SessionOnly:      req.SessionOnly,
	}, true, nil
}

// Refresh rotates a refresh token.
// ok reports whether the refresh token was accepted.
// Refresh 轮换 refresh token。
// ok 表示 refresh token 是否通过校验。
func (s *Service) Refresh(ctx context.Context, refresh string) (RefreshResult, bool, error) {
	ctx = contextutil.Require(ctx)
	if err := s.requireTokens(); err != nil {
		return RefreshResult{}, false, err
	}
	refresh = strings.TrimSpace(refresh)
	if refresh == "" {
		return RefreshResult{}, false, ErrRefreshTokenRequired
	}
	claims, ok, err := s.tokens.ValidateRefreshToken(ctx, refresh)
	if err != nil {
		return RefreshResult{}, false, fmt.Errorf("validate refresh token: %w", err)
	}
	if !ok {
		return RefreshResult{}, false, nil
	}
	principal, ok, err := s.principalForClaims(ctx, claims)
	if err != nil {
		return RefreshResult{}, false, err
	}
	if !ok {
		return RefreshResult{}, false, nil
	}
	result, ok, err := s.tokens.RotateRefreshTokens(ctx, refresh)
	if err != nil {
		return RefreshResult{}, false, fmt.Errorf("rotate refresh token: %w", err)
	}
	if !ok {
		return RefreshResult{}, false, nil
	}
	return RefreshResult{
		Principal:        principal,
		AccessToken:      result.Access,
		AccessExpiresAt:  result.AccessExpiresAt,
		RefreshToken:     result.Refresh,
		RefreshExpiresAt: result.RefreshExpiresAt,
		SessionOnly:      result.SessionOnly,
	}, true, nil
}

// Logout revokes a refresh token.
// Logout 吊销 refresh token。
func (s *Service) Logout(ctx context.Context, refresh string) error {
	ctx = contextutil.Require(ctx)
	if err := s.requireTokens(); err != nil {
		return err
	}
	refresh = strings.TrimSpace(refresh)
	if refresh == "" {
		return ErrRefreshTokenRequired
	}
	if err := s.tokens.RevokeRefresh(ctx, refresh); err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	return nil
}

// AuthenticateBearer validates a user access token or API token and returns its principal.
// AuthenticateBearer 校验用户 access token 或 API token 并返回主体。
func (s *Service) AuthenticateBearer(ctx context.Context, token string) (authcore.Principal, bool, error) {
	ctx = contextutil.Require(ctx)
	if err := s.requireTokens(); err != nil {
		return authcore.Principal{}, false, err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return authcore.Principal{}, false, ErrBearerTokenRequired
	}
	claims, ok, err := s.tokens.ValidateAccessToken(ctx, token)
	if err != nil {
		return authcore.Principal{}, false, fmt.Errorf("validate access token: %w", err)
	}
	if ok {
		return s.principalForClaims(ctx, claims)
	}
	if s.apiTokens == nil {
		return authcore.Principal{}, false, nil
	}
	return s.ValidateAPIToken(ctx, token)
}

// ValidateAPIToken validates an opaque API token.
// ValidateAPIToken 校验 opaque API token。
func (s *Service) ValidateAPIToken(ctx context.Context, raw string) (authcore.Principal, bool, error) {
	ctx = contextutil.Require(ctx)
	if s == nil {
		return authcore.Principal{}, false, ErrServiceMisconfigured
	}
	if s.apiTokens == nil {
		return authcore.Principal{}, false, ErrAPITokenStoreMissing
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return authcore.Principal{}, false, ErrBearerTokenRequired
	}
	now := s.now()
	hash := HashToken(raw)
	token, ok, err := s.apiTokens.GetAPITokenByHash(ctx, hash)
	if err != nil {
		return authcore.Principal{}, false, fmt.Errorf("load api token: %w", err)
	}
	if !ok || token.Hash != hash || !validAPIToken(token, now) {
		return authcore.Principal{}, false, nil
	}
	if err := s.apiTokens.MarkAPITokenUsed(ctx, token.ID, now); err != nil {
		return authcore.Principal{}, false, fmt.Errorf("mark api token used: %w", err)
	}
	return token.Principal(), true, nil
}

// CreateAPIToken creates an opaque API token and stores only its hash.
// CreateAPIToken 创建 opaque API token，并且只存储 hash。
func (s *Service) CreateAPIToken(ctx context.Context, req CreateAPITokenRequest) (CreatedAPIToken, error) {
	if s == nil {
		return CreatedAPIToken{}, ErrServiceMisconfigured
	}
	ctx, err := requireAPITokenStore(ctx, s.apiTokens)
	if err != nil {
		return CreatedAPIToken{}, err
	}
	raw, prefix, hash, err := createRawAPIToken()
	if err != nil {
		return CreatedAPIToken{}, err
	}
	token, err := normalizeAPITokenForStore(req, raw, prefix, hash, s.now())
	if err != nil {
		return CreatedAPIToken{}, err
	}
	if err := s.apiTokens.CreateAPIToken(ctx, token); err != nil {
		return CreatedAPIToken{}, fmt.Errorf("create api token: %w", err)
	}
	publicToken := publicAPIToken(token)
	if err := s.events.tokenCreated(ctx, publicToken); err != nil {
		deleteCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), eventRevokeTimeout)
		defer cancel()
		if _, _, deleteErr := s.apiTokens.DeleteAPIToken(deleteCtx, token.ID); deleteErr != nil {
			return CreatedAPIToken{}, fmt.Errorf("%w: token created hook failed: %w; delete api token: %w", ErrEventFailed, err, deleteErr)
		}
		return CreatedAPIToken{}, fmt.Errorf("%w: token created hook failed: %w", ErrEventFailed, err)
	}
	return CreatedAPIToken{Token: publicToken, Raw: raw}, nil
}

// DeleteAPIToken deletes an API token and emits the revoke hook when a token existed.
// DeleteAPIToken 删除 API token，并在 token 存在时发送吊销 hook。
func (s *Service) DeleteAPIToken(ctx context.Context, id string) (bool, error) {
	if s == nil {
		return false, ErrServiceMisconfigured
	}
	ctx, err := requireAPITokenStore(ctx, s.apiTokens)
	if err != nil {
		return false, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return false, ErrAPITokenIDRequired
	}
	token, ok, err := s.apiTokens.DeleteAPIToken(ctx, id)
	if err != nil {
		return false, fmt.Errorf("delete api token: %w", err)
	}
	if !ok {
		return false, nil
	}
	if err := s.events.tokenRevoked(ctx, publicAPIToken(token)); err != nil {
		return true, fmt.Errorf("%w: token revoked hook failed: %w", ErrEventFailed, err)
	}
	return true, nil
}

func (s *Service) principalForClaims(ctx context.Context, claims authjwt.Claims) (authcore.Principal, bool, error) {
	subject := strings.TrimSpace(claims.Subject)
	if subject == "" {
		return authcore.Principal{}, false, nil
	}
	user, ok, err := s.users.GetUser(ctx, subject)
	if err != nil {
		return authcore.Principal{}, false, fmt.Errorf("load user: %w", err)
	}
	if !ok || user.Disabled {
		return authcore.Principal{}, false, nil
	}
	if strings.TrimSpace(user.ID) == "" {
		return authcore.Principal{}, false, nil
	}
	if user.ID != subject {
		return authcore.Principal{}, false, nil
	}
	return user.Principal(), true, nil
}

func (s *Service) locked(ctx context.Context, key string) (time.Time, bool, error) {
	if s.lockout == nil {
		return time.Time{}, false, nil
	}
	until, locked, err := s.lockout.Check(ctx, key)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("check login lockout: %w", err)
	}
	return until, locked, nil
}

func (s *Service) clearLockout(ctx context.Context, key string) error {
	if s.lockout == nil {
		return nil
	}
	if err := s.lockout.Clear(ctx, key); err != nil {
		return fmt.Errorf("clear login lockout: %w", err)
	}
	return nil
}

func (s *Service) rejectLogin(ctx context.Context, req LoginRequest, userID, reason string) error {
	var lockedUntil time.Time
	var locked bool
	if s.lockout != nil && reason != LoginFailureLocked {
		var err error
		lockedUntil, locked, err = s.lockout.RecordFailure(ctx, req.LockoutKey)
		if err != nil {
			return fmt.Errorf("record login failure: %w", err)
		}
	}
	if err := s.emitLoginFailure(ctx, LoginFailure{
		Username: req.Username,
		UserID:   userID,
		Reason:   reason,
		At:       s.now(),
	}); err != nil {
		return err
	}
	if locked {
		return LockedError{Until: lockedUntil}
	}
	return nil
}

func (s *Service) emitLoginFailure(ctx context.Context, event LoginFailure) error {
	if err := s.events.loginFailure(ctx, event); err != nil {
		return fmt.Errorf("%w: login failure hook failed: %w", ErrEventFailed, err)
	}
	return nil
}

func (s *Service) requireLogin() error {
	if err := s.requireTokens(); err != nil {
		return err
	}
	switch {
	case s.users == nil:
		return ErrUserStoreMissing
	case s.passwords == nil:
		return ErrPasswordVerifierMissing
	default:
		return nil
	}
}

func (s *Service) requireTokens() error {
	switch {
	case s == nil:
		return ErrServiceMisconfigured
	case s.tokens == nil:
		return ErrTokenManagerMissing
	default:
		return nil
	}
}
