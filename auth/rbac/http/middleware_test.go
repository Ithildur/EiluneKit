package rbachttp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	authcore "github.com/Ithildur/EiluneKit/auth"
	corerbac "github.com/Ithildur/EiluneKit/auth/rbac"
	rbachttp "github.com/Ithildur/EiluneKit/auth/rbac/http"
	"github.com/Ithildur/EiluneKit/http/routes"
)

type bearerAuth struct {
	principal authcore.Principal
}

func (a bearerAuth) AuthenticateBearer(ctx context.Context, token string) (authcore.Principal, bool, error) {
	if token != "token" {
		return authcore.Principal{}, false, nil
	}
	return a.principal, true, nil
}

func pveRolePolicy() corerbac.RolePolicy {
	rank := map[string]int{
		"viewer":   1,
		"operator": 2,
		"admin":    3,
	}
	return corerbac.RoleAllows(func(actual string, required string) bool {
		if actual == "vm_user" || required == "vm_user" {
			return actual == required
		}
		actualRank, actualOK := rank[actual]
		requiredRank, requiredOK := rank[required]
		return actualOK && requiredOK && actualRank >= requiredRank
	})
}

func TestMiddlewareRequireRoleAndScopeStoresPrincipal(t *testing.T) {
	mw := rbachttp.NewMiddleware(bearerAuth{principal: authcore.Principal{
		Subject: "user-1",
		Role:    "admin",
		Scopes:  []string{"vm:read"},
		Kind:    authcore.PrincipalKindUser,
	}}, pveRolePolicy())
	nextCalled := false
	handler := mw.RequireRoleAndScope("operator", "vm:read")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		if !routes.Authenticated(r.Context()) {
			t.Fatal("expected routes context to be authenticated")
		}
		principal, ok := authcore.PrincipalFromContext(r.Context())
		if !ok {
			t.Fatal("expected principal in context")
		}
		if principal.Subject != "user-1" || principal.Role != "admin" {
			t.Fatalf("unexpected principal: %#v", principal)
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
}

func TestMiddlewareExistingPrincipalMarksRoutesAuthenticated(t *testing.T) {
	mw := rbachttp.NewMiddleware(nil, nil)
	handler := mw.RequireAuth()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !routes.Authenticated(r.Context()) {
			t.Fatal("expected routes context to be authenticated")
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	ctx := authcore.WithPrincipal(req.Context(), authcore.Principal{
		Subject: "user-1",
		Kind:    authcore.PrincipalKindUser,
	})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req.WithContext(ctx))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
}

func TestMiddlewareRolePolicyKeepsVMUserOutsideViewerHierarchy(t *testing.T) {
	mw := rbachttp.NewMiddleware(bearerAuth{principal: authcore.Principal{
		Subject: "user-1",
		Role:    "vm_user",
		Kind:    authcore.PrincipalKindUser,
	}}, pveRolePolicy())
	handler := mw.RequireRole("viewer")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, rec.Code)
	}
}
