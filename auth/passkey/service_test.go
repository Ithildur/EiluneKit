package passkey_test

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/mldsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	auth "github.com/Ithildur/EiluneKit/auth"
	authhttp "github.com/Ithildur/EiluneKit/auth/http"
	authjwt "github.com/Ithildur/EiluneKit/auth/jwt"
	"github.com/Ithildur/EiluneKit/auth/passkey"
	"github.com/Ithildur/EiluneKit/auth/rbac"
	rbachttp "github.com/Ithildur/EiluneKit/auth/rbac/http"
	"github.com/Ithildur/EiluneKit/auth/store"
	"github.com/Ithildur/EiluneKit/http/routes"
	"github.com/Ithildur/EiluneKit/tools/openapi"
	"github.com/go-chi/chi/v5"
	"github.com/go-webauthn/webauthn/protocol/webauthncbor"
	"github.com/go-webauthn/webauthn/webauthn"
)

const userID = "ba95d114-2cd6-4ca1-a4ee-15038f9e730a"

var errStorage = errors.New("storage unavailable")

type credentialStore struct {
	mu        sync.Mutex
	records   map[string]passkey.Credential
	fail      bool
	failUsage bool
}

func (s *credentialStore) Insert(_ context.Context, c passkey.Credential) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := b64(c.Data.ID)
	if _, ok := s.records[id]; ok {
		return passkey.ErrConflict
	}
	s.records[id] = clone(c)
	return nil
}
func (s *credentialStore) Get(_ context.Context, id string) (passkey.Credential, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return passkey.Credential{}, false, errStorage
	}
	c, ok := s.records[id]
	return clone(c), ok, nil
}
func (s *credentialStore) List(_ context.Context, uid string) ([]passkey.Credential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []passkey.Credential
	for _, c := range s.records {
		if c.UserID == uid {
			result = append(result, clone(c))
		}
	}
	return result, nil
}
func (s *credentialStore) UpdateUsage(_ context.Context, id, uid string, a webauthn.Authenticator, f webauthn.CredentialFlags, at time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failUsage {
		return false, errStorage
	}
	c, ok := s.records[id]
	if !ok || c.UserID != uid {
		return false, nil
	}
	a.SignCount = max(a.SignCount, c.Data.Authenticator.SignCount)
	c.Data.Authenticator = a
	c.Data.Flags = f
	c.LastUsedAt = at
	s.records[id] = clone(c)
	return true, nil
}
func (s *credentialStore) Rename(_ context.Context, id, uid, name string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.records[id]
	if !ok || c.UserID != uid {
		return false, nil
	}
	c.Name = name
	s.records[id] = c
	return true, nil
}
func (s *credentialStore) Delete(_ context.Context, id, uid string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.records[id]
	if !ok || c.UserID != uid {
		return false, nil
	}
	delete(s.records, id)
	return true, nil
}
func clone(c passkey.Credential) passkey.Credential {
	data, err := json.Marshal(c)
	if err != nil {
		panic(err)
	}
	var result passkey.Credential
	if err = json.Unmarshal(data, &result); err != nil {
		panic(err)
	}
	return result
}

type authenticator struct {
	id, public, handle []byte
	sign               func([]byte) []byte
}

func newAuthenticator(t *testing.T, algorithm string) *authenticator {
	t.Helper()
	a := &authenticator{id: []byte(rand.Text())}
	var key map[int]any
	if algorithm == "ES256" {
		private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		key = map[int]any{1: 2, 3: -7, -1: 1, -2: private.X.FillBytes(make([]byte, 32)), -3: private.Y.FillBytes(make([]byte, 32))}
		a.sign = func(data []byte) []byte {
			hash := sha256.Sum256(data)
			sig, err := private.Sign(rand.Reader, hash[:], crypto.SHA256)
			if err != nil {
				t.Fatal(err)
			}
			return sig
		}
	} else {
		private, err := mldsa.GenerateKey(mldsa.MLDSA44())
		if err != nil {
			t.Fatal(err)
		}
		key = map[int]any{1: 7, 3: -48, -1: private.PublicKey().Bytes()}
		a.sign = func(data []byte) []byte {
			sig, err := private.Sign(nil, data, &mldsa.Options{})
			if err != nil {
				t.Fatal(err)
			}
			return sig
		}
	}
	var err error
	a.public, err = webauthncbor.Marshal(key)
	if err != nil {
		t.Fatal(err)
	}
	return a
}
func b64(data []byte) string { return base64.RawURLEncoding.EncodeToString(data) }
func jsonBytes(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func clientData(t *testing.T, kind, challenge, origin string) []byte {
	return jsonBytes(t, map[string]any{"type": kind, "challenge": challenge, "origin": origin, "crossOrigin": false})
}
func authData(flags byte) []byte {
	hash := sha256.Sum256([]byte("example.com"))
	return append(hash[:], flags, 0, 0, 0, 0)
}
func (a *authenticator) registration(t *testing.T, begin passkey.Registration, origin string) []byte {
	t.Helper()
	data := jsonBytes(t, begin.Options)
	var options struct {
		PublicKey struct {
			Challenge string `json:"challenge"`
			User      struct {
				ID string `json:"id"`
			} `json:"user"`
		} `json:"publicKey"`
	}
	if err := json.Unmarshal(data, &options); err != nil {
		t.Fatal(err)
	}
	var err error
	a.handle, err = base64.RawURLEncoding.DecodeString(options.PublicKey.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	ad := append(authData(0x45), make([]byte, 16)...)
	ad = binary.BigEndian.AppendUint16(ad, uint16(len(a.id)))
	ad = append(ad, a.id...)
	ad = append(ad, a.public...)
	att, err := webauthncbor.Marshal(map[string]any{"fmt": "none", "attStmt": map[string]any{}, "authData": ad})
	if err != nil {
		t.Fatal(err)
	}
	return jsonBytes(t, map[string]any{"id": b64(a.id), "rawId": b64(a.id), "type": "public-key", "response": map[string]any{
		"clientDataJSON": b64(clientData(t, "webauthn.create", options.PublicKey.Challenge, origin)), "attestationObject": b64(att), "transports": []string{"internal"}}, "clientExtensionResults": map[string]any{}})
}
func (a *authenticator) assertion(t *testing.T, begin passkey.Login, origin string) []byte {
	t.Helper()
	data := jsonBytes(t, begin.Options)
	var options struct {
		PublicKey struct {
			Challenge string `json:"challenge"`
		} `json:"publicKey"`
	}
	if err := json.Unmarshal(data, &options); err != nil {
		t.Fatal(err)
	}
	client := clientData(t, "webauthn.get", options.PublicKey.Challenge, origin)
	hash := sha256.Sum256(client)
	ad := authData(0x05)
	message := append(bytes.Clone(ad), hash[:]...)
	return jsonBytes(t, map[string]any{"id": b64(a.id), "rawId": b64(a.id), "type": "public-key", "response": map[string]any{
		"clientDataJSON": b64(client), "authenticatorData": b64(ad), "signature": b64(a.sign(message)), "userHandle": b64(a.handle)}, "clientExtensionResults": map[string]any{}})
}
func newService(t *testing.T) (*passkey.Service, *credentialStore) {
	t.Helper()
	storage := &credentialStore{records: make(map[string]passkey.Credential)}
	s, err := passkey.New(passkey.Options{RPID: "example.com", RPDisplayName: "Example", Origins: []string{"https://example.com"}, Store: storage, Flows: passkey.NewMemoryFlowStore(100),
		LookupUser: func(_ context.Context, id string) (passkey.User, bool, error) {
			return passkey.User{ID: id, Name: "alice", DisplayName: "Alice"}, true, nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	return s, storage
}
func register(t *testing.T, s *passkey.Service, a *authenticator) passkey.Info {
	t.Helper()
	begin, err := s.BeginRegistration(t.Context(), userID)
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.FinishRegistration(t.Context(), userID, begin.FlowID, "Laptop", a.registration(t, begin, "https://example.com"))
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestCeremonyBoundaries(t *testing.T) {
	for _, algorithm := range []string{"ES256", "ML-DSA-44"} {
		t.Run(algorithm, func(t *testing.T) {
			s, storage := newService(t)
			a := newAuthenticator(t, algorithm)
			info := register(t, s, a)
			begin, err := s.BeginLogin(t.Context(), true)
			if err != nil {
				t.Fatal(err)
			}
			assertion := a.assertion(t, begin, "https://example.com")
			result, err := s.FinishLogin(t.Context(), begin.FlowID, assertion)
			if err != nil || result.User.ID != userID || !result.SessionOnly {
				t.Fatalf("login: %+v %v", result, err)
			}
			if _, err = s.FinishLogin(t.Context(), begin.FlowID, assertion); !errors.Is(err, passkey.ErrRejected) {
				t.Fatalf("replay: %v", err)
			}
			for _, scenario := range []string{"origin", "signature", "owner", "deleted", "storage"} {
				t.Run(scenario, func(t *testing.T) {
					begin, err := s.BeginLogin(t.Context(), false)
					if err != nil {
						t.Fatal(err)
					}
					origin := "https://example.com"
					if scenario == "origin" {
						origin = "https://attacker.example"
					}
					if scenario == "owner" {
						old := a.handle
						a.handle = []byte("other-user")
						defer func() { a.handle = old }()
					}
					assertion := a.assertion(t, begin, origin)
					if scenario == "signature" {
						var value map[string]any
						if err = json.Unmarshal(assertion, &value); err != nil {
							t.Fatal(err)
						}
						value["response"].(map[string]any)["signature"] = b64(make([]byte, 64))
						assertion = jsonBytes(t, value)
					}
					if scenario == "deleted" {
						saved, _, _ := storage.Get(t.Context(), info.ID)
						_, _ = storage.Delete(t.Context(), info.ID, userID)
						defer func() { _ = storage.Insert(t.Context(), saved) }()
					}
					if scenario == "storage" {
						storage.fail = true
						defer func() { storage.fail = false }()
					}
					_, err = s.FinishLogin(t.Context(), begin.FlowID, assertion)
					want := passkey.ErrRejected
					if scenario == "storage" {
						want = errStorage
					}
					if !errors.Is(err, want) {
						t.Fatalf("want %v got %v", want, err)
					}
					if _, err = s.FinishLogin(t.Context(), begin.FlowID, assertion); !errors.Is(err, passkey.ErrRejected) {
						t.Fatalf("failed flow reusable: %v", err)
					}
				})
			}
			beginReg, err := s.BeginRegistration(t.Context(), userID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.FinishRegistration(t.Context(), "other", beginReg.FlowID, "Other", a.registration(t, beginReg, "https://example.com")); !errors.Is(err, passkey.ErrRejected) {
				t.Fatalf("registration owner: %v", err)
			}
			if err = s.Rename(t.Context(), "other", info.ID, "Stolen"); !errors.Is(err, passkey.ErrNotFound) {
				t.Fatalf("rename owner: %v", err)
			}
			if err = s.Delete(t.Context(), "other", info.ID); !errors.Is(err, passkey.ErrNotFound) {
				t.Fatalf("delete owner: %v", err)
			}
			if err = s.Rename(t.Context(), userID, info.ID, "Phone"); err != nil {
				t.Fatal(err)
			}
			items, err := s.List(t.Context(), userID)
			if err != nil || len(items) != 1 || items[0].Name != "Phone" || items[0].LastUsedAt.IsZero() {
				t.Fatalf("metadata: %+v %v", items, err)
			}
		})
	}
}

func TestMemoryFlowConsumption(t *testing.T) {
	s := passkey.NewMemoryFlowStore(1)
	ctx := t.Context()
	flow := passkey.Flow{Operation: "login", Session: webauthn.SessionData{Expires: time.Now().Add(time.Minute)}}
	if err := s.Create(ctx, "once", flow); err != nil {
		t.Fatal(err)
	}
	if err := s.Create(ctx, "once", flow); !errors.Is(err, passkey.ErrConflict) {
		t.Fatal(err)
	}
	if err := s.Create(ctx, "full", flow); !errors.Is(err, passkey.ErrCapacity) {
		t.Fatal(err)
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			_, ok, err := s.Consume(ctx, "once")
			if err != nil {
				t.Error(err)
			}
			if ok {
				successes.Add(1)
			}
		})
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("consumed %d times", successes.Load())
	}
	flow.Session.Expires = time.Now().Add(-time.Second)
	if err := s.Create(ctx, "expired", flow); !errors.Is(err, passkey.ErrRejected) {
		t.Fatal(err)
	}
}

type users struct{ user rbac.User }

func (u *users) GetUser(_ context.Context, id string) (rbac.User, bool, error) {
	return u.user, u.user.ID == id, nil
}
func (u *users) GetUserByUsername(_ context.Context, name string) (rbac.User, bool, error) {
	return u.user, u.user.Username == name, nil
}

type tokenManager struct {
	*authjwt.Manager
	issued      int
	lastRefresh string
}

func (m *tokenManager) IssueSessionTokens(ctx context.Context, id string, opts auth.IssueOptions) (string, time.Time, string, time.Time, error) {
	a, ae, r, re, err := m.Manager.IssueSessionTokens(ctx, id, opts)
	m.issued++
	m.lastRefresh = r
	return a, ae, r, re, err
}

func TestHTTPAdapters(t *testing.T) {
	for _, test := range []struct{ adapter, algorithm string }{
		{"auth", "ES256"},
		{"rbac", "ML-DSA-44"},
	} {
		adapter, algorithm := test.adapter, test.algorithm
		t.Run(adapter+"/"+algorithm, func(t *testing.T) {
			ctx := t.Context()
			directory := &users{user: rbac.User{ID: userID, Username: "alice", Role: "reader", Scopes: []string{"read"}}}
			manager, err := authjwt.New("0123456789abcdef0123456789abcdef", store.NewMemoryStore())
			if err != nil {
				t.Fatal(err)
			}
			tokens := &tokenManager{Manager: manager}
			access, _, _, _, err := tokens.IssueSessionTokens(ctx, userID, auth.IssueOptions{})
			if err != nil {
				t.Fatal(err)
			}
			storage := &credentialStore{records: make(map[string]passkey.Credential)}
			lookup := func(_ context.Context, id string) (passkey.User, bool, error) {
				return passkey.User{ID: id, Name: "alice", DisplayName: "Alice"}, id == userID && !directory.user.Disabled, nil
			}
			eventFailure := false
			event := func() error {
				if eventFailure {
					return errors.New("event failed")
				}
				return nil
			}
			var rbacService *rbac.Service
			if adapter == "rbac" {
				rbacService, err = rbac.NewService(rbac.ServiceOptions{Users: directory, Tokens: tokens, Passwords: rbac.PasswordVerifierFunc(func(context.Context, rbac.User, string) (bool, error) {
					t.Error("password verification called")
					return false, nil
				}), Events: rbac.Events{OnLoginSuccess: func(context.Context, auth.Principal) error { return event() }}})
				if err != nil {
					t.Fatal(err)
				}
				lookup = rbacService.PasskeyUser
			}
			keys, err := passkey.New(passkey.Options{RPID: "example.com", RPDisplayName: "Example", Origins: []string{"https://example.com"}, Store: storage, Flows: passkey.NewMemoryFlowStore(100), LookupUser: lookup})
			if err != nil {
				t.Fatal(err)
			}
			router := chi.NewRouter()
			var routeList []routes.Route
			basePath := "/tenants/{tenantID}/auth"
			if adapter == "auth" {
				h, err := authhttp.NewHandler(tokens, authhttp.Options{BasePath: new(basePath), Passkeys: keys, RateLimit: &authhttp.RateLimitOptions{Disabled: true}, LoginAuthenticator: auth.LoginAuthenticatorFunc(func(context.Context, string, string) (string, bool, error) {
					t.Error("password verification called")
					return "", false, nil
				}), Events: authhttp.Events{Login: func(context.Context, authhttp.LoginEvent) error { return event() }}})
				if err != nil {
					t.Fatal(err)
				}
				routeList = h.Routes()
			} else {
				h, err := rbachttp.NewHandler(rbacService, rbachttp.Options{BasePath: new(basePath), Passkeys: keys, RateLimit: &rbachttp.RateLimitOptions{Disabled: true}})
				if err != nil {
					t.Fatal(err)
				}
				routeList = h.Routes()
			}
			if _, err := openapi.Generate(routeList, openapi.Options{Title: "Auth API", Version: "1"}); err != nil {
				t.Fatal(err)
			}
			if err := routes.Mount(router, "", routeList); err != nil {
				t.Fatal(err)
			}
			call := func(method, path string, body any, bearer string) *httptest.ResponseRecorder {
				r := httptest.NewRequest(method, "https://example.com/tenants/acme"+path, bytes.NewReader(jsonBytes(t, body)))
				r.Header.Set("Content-Type", "application/json")
				if bearer != "" {
					r.Header.Set("Authorization", "Bearer "+bearer)
				}
				w := httptest.NewRecorder()
				router.ServeHTTP(w, r)
				return w
			}
			post := func(body any, bearer string) *httptest.ResponseRecorder {
				return call("POST", "/auth/passkey", body, bearer)
			}
			decode := func(w *httptest.ResponseRecorder, out any) {
				t.Helper()
				if w.Code != 200 {
					t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
				}
				if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
					t.Fatal(err)
				}
			}
			if w := post(map[string]any{"operation": "register.begin"}, ""); w.Code != 401 {
				t.Fatalf("unauthenticated binding: %d", w.Code)
			}
			var begin passkey.Registration
			decode(post(map[string]any{"operation": "register.begin"}, access), &begin)
			a := newAuthenticator(t, algorithm)
			var info passkey.Info
			decode(post(map[string]any{"operation": "register.finish", "flow_id": begin.FlowID, "name": "Laptop", "credential": jsontext.Value(a.registration(t, begin, "https://example.com"))}, access), &info)
			for _, scenario := range []string{"success", "disabled", "storage", "event"} {
				t.Run(scenario, func(t *testing.T) {
					var login passkey.Login
					decode(post(map[string]any{"operation": "login.begin", "persistence": "session"}, ""), &login)
					assertion := a.assertion(t, login, "https://example.com")
					directory.user.Disabled = scenario == "disabled"
					storage.failUsage = scenario == "storage"
					eventFailure = scenario == "event"
					directory.user.Role = "editor"
					before := tokens.issued
					w := post(map[string]any{"operation": "login.finish", "flow_id": login.FlowID, "credential": jsontext.Value(assertion)}, "")
					if scenario == "success" {
						var result struct {
							Access      string         `json:"access_token"`
							Refresh     string         `json:"refresh_token"`
							User        auth.Principal `json:"user"`
							SessionOnly bool           `json:"session_only"`
						}
						decode(w, &result)
						claims, ok, err := tokens.ValidateAccessToken(ctx, result.Access)
						if err != nil || !ok || claims.Subject != userID {
							t.Fatalf("access token: %v %v", ok, err)
						}
						if adapter == "auth" {
							if result.Refresh != "" || len(w.Result().Cookies()) != 2 {
								t.Fatalf("cookie contract: %s", w.Body.String())
							}
							for _, c := range w.Result().Cookies() {
								if c.MaxAge != 0 || !c.Expires.IsZero() {
									t.Fatal("session cookie persisted")
								}
							}
						} else {
							if result.Refresh == "" || result.User.Role != "editor" || !result.SessionOnly || len(w.Result().Cookies()) != 0 {
								t.Fatalf("RBAC contract: %s", w.Body.String())
							}
						}
						if replay := post(map[string]any{"operation": "login.finish", "flow_id": login.FlowID, "credential": jsontext.Value(assertion)}, ""); replay.Code != 401 {
							t.Fatalf("HTTP replay: %d", replay.Code)
						}
					} else {
						if scenario == "disabled" {
							if w.Code != 401 {
								t.Fatalf("disabled: %d %s", w.Code, w.Body.String())
							}
						} else if w.Code < 500 {
							t.Fatalf("failure concealed: %d %s", w.Code, w.Body.String())
						}
						if len(w.Result().Cookies()) != 0 {
							t.Fatal("cookies on failure")
						}
						if scenario == "event" {
							if tokens.issued != before+1 {
								t.Fatal("event did not run after issuance")
							}
							_, ok, err := tokens.ValidateRefreshToken(ctx, tokens.lastRefresh)
							if err != nil || ok {
								t.Fatalf("session not revoked: %v %v", ok, err)
							}
						} else if tokens.issued != before {
							t.Fatal("issued session on failure")
						}
					}
					directory.user.Disabled = false
					storage.failUsage = false
					eventFailure = false
				})
			}
			if w := call("PATCH", "/auth/passkeys/"+info.ID, map[string]string{"name": "Phone"}, access); w.Code != 204 {
				t.Fatalf("rename: %d %s", w.Code, w.Body.String())
			}
			var listed struct {
				Credentials []passkey.Info `json:"credentials"`
			}
			decode(call("GET", "/auth/passkeys", nil, access), &listed)
			if len(listed.Credentials) != 1 || listed.Credentials[0].Name != "Phone" {
				t.Fatal("list metadata")
			}
			if w := call("DELETE", "/auth/passkeys/"+info.ID, nil, access); w.Code != 204 {
				t.Fatal(w.Code, w.Body.String())
			}
			if _, ok, err := tokens.ValidateAccessToken(ctx, access); err != nil || !ok {
				t.Fatal("credential deletion revoked session")
			}
		})
	}
}
