package authhttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Ithildur/EiluneKit/auth/http"
	authjwt "github.com/Ithildur/EiluneKit/auth/jwt"
	"github.com/Ithildur/EiluneKit/http/response"
)

type bearerOnlyStub struct {
	claims authjwt.Claims
	ok     bool
	err    error
	token  string
}

func (s *bearerOnlyStub) ValidateAccessToken(_ context.Context, token string) (authjwt.Claims, bool, error) {
	s.token = token
	if s.ok {
		return s.claims, true, nil
	}
	return authjwt.Claims{}, false, s.err
}

func TestRequireBearerAcceptsMinimalValidatorInterface(t *testing.T) {
	auth := &bearerOnlyStub{
		ok: true,
		claims: authjwt.Claims{
			SessionID: "session-1",
		},
	}
	middleware, err := authhttp.RequireBearer(auth)
	if err != nil {
		t.Fatalf("require bearer: %v", err)
	}
	nextCalled := false

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		claims, ok := authjwt.ClaimsFromContext(r.Context())
		if !ok {
			t.Fatal("expected claims in request context")
		}
		if got, want := claims.SessionID, "session-1"; got != want {
			t.Fatalf("expected session id %q, got %q", want, got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !nextCalled {
		t.Fatal("expected next handler to be called")
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
	if got, want := auth.token, "token"; got != want {
		t.Fatalf("expected token %q, got %q", want, got)
	}
}

func TestRequireBearerRejectsInvalidHeader(t *testing.T) {
	auth := &bearerOnlyStub{}
	middleware, err := authhttp.RequireBearer(auth)
	if err != nil {
		t.Fatalf("require bearer: %v", err)
	}

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if auth.token != "" {
		t.Fatalf("expected validator to stay untouched, got token %q", auth.token)
	}
	assertAuthErrorResponse(t, rec, http.StatusUnauthorized, "unauthorized", "missing or invalid bearer token")
}

func TestRequireBearerRejectsInvalidToken(t *testing.T) {
	auth := &bearerOnlyStub{}
	middleware, err := authhttp.RequireBearer(auth)
	if err != nil {
		t.Fatalf("require bearer: %v", err)
	}
	nextCalled := false

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		t.Fatal("next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if nextCalled {
		t.Fatal("expected next handler to stay untouched")
	}
	assertAuthErrorResponse(t, rec, http.StatusUnauthorized, "unauthorized", "token invalid or expired")
}

func TestRequireBearerPropagatesStoreUnavailable(t *testing.T) {
	auth := &bearerOnlyStub{
		err: authjwt.ErrStoreUnavailable,
	}
	middleware, err := authhttp.RequireBearer(auth)
	if err != nil {
		t.Fatalf("require bearer: %v", err)
	}
	nextCalled := false

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		t.Fatal("next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if nextCalled {
		t.Fatal("expected next handler to stay untouched")
	}
	assertAuthErrorResponse(t, rec, http.StatusServiceUnavailable, "auth_unavailable", "auth is unavailable")
}

func TestRequireBearerRejectsMisconfiguredValidator(t *testing.T) {
	auth := &bearerOnlyStub{
		err: authjwt.ErrManagerMisconfigured,
	}
	middleware, err := authhttp.RequireBearer(auth)
	if err != nil {
		t.Fatalf("require bearer: %v", err)
	}
	nextCalled := false

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		t.Fatal("next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if nextCalled {
		t.Fatal("expected next handler to stay untouched")
	}
	assertAuthErrorResponse(t, rec, http.StatusInternalServerError, "auth_misconfigured", "auth is misconfigured")
}

func TestOptionalBearerAllowsMissingToken(t *testing.T) {
	auth := &bearerOnlyStub{}
	middleware, err := authhttp.OptionalBearer(auth)
	if err != nil {
		t.Fatalf("optional bearer: %v", err)
	}
	nextCalled := false

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/public", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !nextCalled {
		t.Fatal("expected next handler to be called")
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
	if auth.token != "" {
		t.Fatalf("expected validator to stay untouched, got token %q", auth.token)
	}
}

func TestOptionalBearerRejectsInvalidToken(t *testing.T) {
	auth := &bearerOnlyStub{}
	middleware, err := authhttp.OptionalBearer(auth)
	if err != nil {
		t.Fatalf("optional bearer: %v", err)
	}

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/public", nil)
	req.Header.Set("Authorization", "Bearer bad")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assertAuthErrorResponse(t, rec, http.StatusUnauthorized, "unauthorized", "token invalid or expired")
}

func TestRequireAPIKeyAcceptsCustomHeader(t *testing.T) {
	middleware, err := authhttp.RequireAPIKey(authhttp.APIKeyValidatorFunc(func(ctx context.Context, key string) (bool, error) {
		return key == "secret", nil
	}), "X-Node-Secret")
	if err != nil {
		t.Fatalf("require api key: %v", err)
	}
	nextCalled := false

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/node/metrics", nil)
	req.Header.Set("X-Node-Secret", "secret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !nextCalled {
		t.Fatal("expected next handler to be called")
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
}

func TestRequireAPIKeyRejectsMissingKey(t *testing.T) {
	middleware, err := authhttp.RequireAPIKey(authhttp.APIKeyValidatorFunc(func(ctx context.Context, key string) (bool, error) {
		return true, nil
	}), "")
	if err != nil {
		t.Fatalf("require api key: %v", err)
	}

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/node/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assertAuthErrorResponse(t, rec, http.StatusUnauthorized, "unauthorized", "missing api key")
}

func assertAuthErrorResponse(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantCode, wantMsg string) {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("expected status %d, got %d", wantStatus, rec.Code)
	}

	var payload response.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	if got, want := payload.Code, wantCode; got != want {
		t.Fatalf("expected code %q, got %q", want, got)
	}
	if got, want := payload.Message, wantMsg; got != want {
		t.Fatalf("expected message %q, got %q", want, got)
	}
}
