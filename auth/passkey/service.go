package passkey

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/protocol/webauthncose"
	"github.com/go-webauthn/webauthn/webauthn"
)

// Service verifies passkeys without issuing sessions. Construct with New.
// Service 验证通行密钥，不签发会话。使用 New 构造。
type Service struct {
	web     *webauthn.WebAuthn
	store   Store
	flows   FlowStore
	lookup  func(context.Context, string) (User, bool, error)
	timeout time.Duration
}

// New constructs a service requiring discoverable credentials and user verification.
// New 构造要求可发现凭据及用户验证的服务。
func New(opts Options) (*Service, error) {
	if opts.RPID == "" || strings.TrimSpace(opts.RPDisplayName) == "" {
		return nil, fmt.Errorf("passkey: RPID and RPDisplayName are required")
	}
	if opts.Store == nil || opts.Flows == nil || opts.LookupUser == nil {
		return nil, fmt.Errorf("passkey: Store, Flows and LookupUser are required")
	}
	if opts.Timeout == 0 {
		opts.Timeout = 5 * time.Minute
	}
	if opts.Timeout < 0 {
		return nil, fmt.Errorf("passkey: Timeout must be positive")
	}
	w, err := webauthn.New(&webauthn.Config{
		RPID: opts.RPID, RPDisplayName: opts.RPDisplayName, RPOrigins: slices.Clone(opts.Origins),
		AttestationPreference: protocol.PreferNoAttestation,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey:        protocol.ResidentKeyRequirementRequired,
			RequireResidentKey: new(true), UserVerification: protocol.VerificationRequired,
		},
		Timeouts: webauthn.TimeoutsConfig{
			Login:        webauthn.TimeoutConfig{Enforce: true, Timeout: opts.Timeout, TimeoutUVD: opts.Timeout},
			Registration: webauthn.TimeoutConfig{Enforce: true, Timeout: opts.Timeout, TimeoutUVD: opts.Timeout},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("passkey config: %w", err)
	}
	return &Service{web: w, store: opts.Store, flows: opts.Flows, lookup: opts.LookupUser, timeout: opts.Timeout}, nil
}

// BeginRegistration starts binding a credential. The caller must authorize userID.
// BeginRegistration 开始绑定凭据。调用方必须已授权 userID。
func (s *Service) BeginRegistration(ctx context.Context, userID string) (Registration, error) {
	u, err := s.user(ctx, userID)
	if err != nil {
		return Registration{}, err
	}
	records, err := s.store.List(ctx, userID)
	if err != nil {
		return Registration{}, fmt.Errorf("list passkeys: %w", err)
	}
	exclude := make([]protocol.CredentialDescriptor, 0, len(records))
	for _, record := range records {
		exclude = append(exclude, record.Data.Descriptor())
	}
	options, session, err := s.web.BeginRegistration(s.webUser(u, nil), webauthn.WithExclusions(exclude),
		webauthn.WithCredentialParameters([]protocol.CredentialParameter{
			{Type: protocol.PublicKeyCredentialType, Algorithm: webauthncose.AlgMLDSA44},
			{Type: protocol.PublicKeyCredentialType, Algorithm: webauthncose.AlgES256},
		}))
	if err != nil {
		return Registration{}, fmt.Errorf("begin registration: %w", err)
	}
	id, err := s.saveFlow(ctx, Flow{Operation: "register", UserID: userID, Session: *session})
	return Registration{FlowID: id, Options: options}, err
}

// FinishRegistration consumes a flow and saves the verified credential.
// The caller must authorize the same userID as BeginRegistration.
// FinishRegistration 消费流程并保存验证后的凭据。
// 调用方必须授权与 BeginRegistration 相同的 userID。
func (s *Service) FinishRegistration(ctx context.Context, userID, flowID, name string, response []byte) (Info, error) {
	flow, err := s.consume(ctx, flowID, "register")
	if err != nil {
		return Info{}, err
	}
	if flow.UserID != userID {
		return Info{}, ErrRejected
	}
	name, err = credentialName(name)
	if err != nil {
		return Info{}, err
	}
	u, err := s.user(ctx, userID)
	if err != nil {
		return Info{}, err
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(response)
	if err != nil {
		return Info{}, fmt.Errorf("%w: %w", ErrRejected, err)
	}
	data, err := s.web.CreateCredential(s.webUser(u, nil), flow.Session, parsed)
	if err != nil {
		return Info{}, fmt.Errorf("%w: %w", ErrRejected, err)
	}
	record := Credential{UserID: userID, Name: name, CreatedAt: time.Now().UTC(), Data: *data}
	if err = s.store.Insert(ctx, record); err != nil {
		return Info{}, fmt.Errorf("insert passkey: %w", err)
	}
	return info(record), nil
}

// BeginLogin starts a username-less login and binds its session preference.
// BeginLogin 开始无用户名登录并绑定会话偏好。
func (s *Service) BeginLogin(ctx context.Context, sessionOnly bool) (Login, error) {
	options, session, err := s.web.BeginDiscoverableLogin()
	if err != nil {
		return Login{}, fmt.Errorf("begin passkey login: %w", err)
	}
	id, err := s.saveFlow(ctx, Flow{Operation: "login", SessionOnly: sessionOnly, Session: *session})
	return Login{FlowID: id, Options: options}, err
}

// FinishLogin consumes a flow, verifies the assertion, checks the current account,
// and persists usage before returning an identity. It never issues tokens.
// FinishLogin 消费流程、验证断言、检查当前账户并保存使用状态后返回身份，不签发 token。
func (s *Service) FinishLogin(ctx context.Context, flowID string, response []byte) (LoginResult, error) {
	flow, err := s.consume(ctx, flowID, "login")
	if err != nil {
		return LoginResult{}, err
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(response)
	if err != nil {
		return LoginResult{}, fmt.Errorf("%w: %w", ErrRejected, err)
	}
	id := base64.RawURLEncoding.EncodeToString(parsed.RawID)
	record, found, err := s.store.Get(ctx, id)
	if err != nil {
		return LoginResult{}, fmt.Errorf("load passkey: %w", err)
	}
	if !found {
		return LoginResult{}, ErrRejected
	}
	u := s.webUser(User{ID: record.UserID}, []webauthn.Credential{record.Data})
	_, data, err := s.web.ValidatePasskeyLogin(func(_, _ []byte) (webauthn.User, error) {
		return u, nil
	}, flow.Session, parsed)
	if err != nil {
		return LoginResult{}, fmt.Errorf("%w: %w", ErrRejected, err)
	}
	user, err := s.user(ctx, record.UserID)
	if err != nil {
		return LoginResult{}, err
	}
	updated, err := s.store.UpdateUsage(ctx, id, record.UserID, data.Authenticator, data.Flags, time.Now().UTC())
	if err != nil {
		return LoginResult{}, fmt.Errorf("update passkey usage: %w", err)
	}
	if !updated {
		return LoginResult{}, ErrRejected
	}
	return LoginResult{User: user, SessionOnly: flow.SessionOnly}, nil
}

// List returns metadata for an authorized user's credentials.
// List 返回已授权用户的凭据元数据。
func (s *Service) List(ctx context.Context, userID string) ([]Info, error) {
	if _, err := s.user(ctx, userID); err != nil {
		return nil, err
	}
	records, err := s.store.List(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list passkeys: %w", err)
	}
	items := make([]Info, 0, len(records))
	for _, record := range records {
		items = append(items, info(record))
	}
	return items, nil
}

// Rename changes an authorized user's credential name.
// Rename 修改已授权用户的凭据名称。
func (s *Service) Rename(ctx context.Context, userID, id, name string) error {
	if _, err := s.user(ctx, userID); err != nil {
		return err
	}
	name, err := credentialName(name)
	if err != nil {
		return err
	}
	ok, err := s.store.Rename(ctx, id, userID, name)
	if err != nil {
		return fmt.Errorf("rename passkey: %w", err)
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

// Delete removes an authorized user's credential without revoking sessions.
// Delete 删除已授权用户的凭据，不撤销现有会话。
func (s *Service) Delete(ctx context.Context, userID, id string) error {
	if _, err := s.user(ctx, userID); err != nil {
		return err
	}
	ok, err := s.store.Delete(ctx, id, userID)
	if err != nil {
		return fmt.Errorf("delete passkey: %w", err)
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

func (s *Service) user(ctx context.Context, id string) (User, error) {
	u, ok, err := s.lookup(ctx, id)
	if err != nil {
		return User{}, fmt.Errorf("load passkey user: %w", err)
	}
	if !ok {
		return User{}, ErrRejected
	}
	if id == "" || id != strings.TrimSpace(id) || u.ID != id {
		return User{}, fmt.Errorf("passkey: invalid user identity")
	}
	return u, nil
}

func (s *Service) saveFlow(ctx context.Context, flow Flow) (string, error) {
	id := rand.Text()
	flow.RPID = s.web.Config.RPID
	flow.Session.Expires = time.Now().Add(s.timeout)
	if err := s.flows.Create(ctx, id, flow); err != nil {
		return "", fmt.Errorf("create passkey flow: %w", err)
	}
	return id, nil
}

func (s *Service) consume(ctx context.Context, id, operation string) (Flow, error) {
	flow, found, err := s.flows.Consume(ctx, id)
	if err != nil {
		return Flow{}, fmt.Errorf("consume passkey flow: %w", err)
	}
	if !found || flow.Operation != operation || flow.RPID != s.web.Config.RPID {
		return Flow{}, ErrRejected
	}
	return flow, nil
}

func credentialName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 100 {
		return "", ErrInvalidName
	}
	return name, nil
}

func info(c Credential) Info {
	return Info{ID: base64.RawURLEncoding.EncodeToString(c.Data.ID), Name: c.Name, CreatedAt: c.CreatedAt, LastUsedAt: c.LastUsedAt}
}

type webUser struct {
	User
	handle      []byte
	credentials []webauthn.Credential
}

func (s *Service) webUser(u User, credentials []webauthn.Credential) webUser {
	// Domain separation keeps the stable handle scoped to this RP.
	// 域分离使稳定的用户句柄限定在当前 RP 范围内。
	handle := sha256.Sum256([]byte("EiluneKit/passkey/v1\x00" + s.web.Config.RPID + "\x00" + u.ID))
	return webUser{User: u, handle: handle[:], credentials: credentials}
}

func (u webUser) WebAuthnID() []byte                         { return u.handle }
func (u webUser) WebAuthnName() string                       { return u.Name }
func (u webUser) WebAuthnDisplayName() string                { return u.DisplayName }
func (u webUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }
