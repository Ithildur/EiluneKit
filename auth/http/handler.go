package authhttp

import (
	"errors"
	"fmt"
	stdhttp "net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/Ithildur/EiluneKit/auth"
	authjwt "github.com/Ithildur/EiluneKit/auth/jwt"
	authsession "github.com/Ithildur/EiluneKit/auth/session"
	"github.com/Ithildur/EiluneKit/http/decoder"
	"github.com/Ithildur/EiluneKit/http/middleware"
	"github.com/Ithildur/EiluneKit/http/response"
	"github.com/Ithildur/EiluneKit/http/routes"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Handler serves auth endpoints.
// Handler 提供认证端点。
type Handler struct {
	auth    TokenManager
	service *auth.Service
	options Options
	bearer  routes.Middleware
}

const errAuthMisconfiguredCode = "auth_misconfigured"
const errAuthMisconfiguredMessage = "auth is misconfigured"

// NewHandler returns a Handler.
// Call NewHandler(manager, opts).
// Zero-valued options fall back to DefaultOptions.
// NewHandler 返回 Handler。
// 调用 NewHandler(manager, opts)。
// 零值选项会回退到 DefaultOptions。
func NewHandler(manager TokenManager, opts Options) (*Handler, error) {
	effective := DefaultOptions()
	effective = applyOptions(effective, opts)
	service, err := auth.New(manager, effective.LoginAuthenticator)
	if err != nil {
		return nil, err
	}
	bearer, err := RequireBearer(manager)
	if err != nil {
		return nil, fmt.Errorf("build bearer middleware: %w", err)
	}

	return &Handler{
		auth:    manager,
		service: service,
		options: effective,
		bearer:  bearer,
	}, nil
}

// Register mounts the auth routes on r.
// Call Register(r) to mount routes with their auth middleware.
// Register 在 r 上挂载认证路由。
// 调用 Register(r) 挂载路由及其认证中间件。
func (h *Handler) Register(r chi.Router) error {
	if h == nil {
		return fmt.Errorf("authhttp: handler is nil")
	}
	return routes.Mount(r, "", h.Routes())
}

// Routes returns the auth routes.
// Returned routes already include their auth middleware.
// Nil Handler is invalid and panics.
// Routes 返回认证路由。
// 返回的路由已包含各自的认证中间件。
// Nil Handler 无效，调用时会 panic。
func (h *Handler) Routes() []routes.Route {
	if h == nil {
		panic("authhttp: handler is nil")
	}

	refresh := h.requireRefreshCookie()

	opts := h.options
	var loginChain []func(stdhttp.Handler) stdhttp.Handler
	rateLimitOpts := opts.RateLimit
	if rateLimitOpts != nil && len(rateLimitOpts.TrustedProxies) == 0 && rateLimitOpts.KeyFunc == nil && len(opts.TrustedProxies) > 0 {
		cloned := *rateLimitOpts
		cloned.TrustedProxies = append([]netip.Prefix(nil), opts.TrustedProxies...)
		rateLimitOpts = &cloned
	}
	if rate := LoginRateLimit(rateLimitOpts); rate != nil {
		loginChain = append(loginChain, rate)
	}
	maxBytes := opts.MaxBodyBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxBodyBytes
	}
	loginChain = append(loginChain, middleware.LimitBody(maxBytes))

	authRoutes := routes.NewBlueprint(
		routes.DefaultTags("auth"),
		routes.DefaultAuth(routes.AuthPublic),
	)
	authRoutes.Post(
		"/login",
		"Login",
		routes.Func(h.handleLogin),
		routes.Use(loginChain...),
		routes.Use(middleware.RequireJSONBody),
	)
	authRoutes.Post(
		"/refresh",
		"Refresh access token",
		routes.Func(h.handleRefresh),
		routes.Auth(routes.AuthRequired),
		routes.Use(refresh),
	)
	authRoutes.Post(
		"/logout",
		"Logout",
		routes.Func(h.handleLogout),
		routes.Auth(routes.AuthRequired),
		routes.Use(refresh),
	)

	sessions := routes.NewBlueprint(
		routes.DefaultAuth(routes.AuthRequired),
		routes.DefaultMiddleware(h.bearer),
	)
	sessions.Delete(
		"/current",
		"Revoke current session",
		routes.Func(h.handleDeleteCurrentSession),
	)
	sessions.Delete(
		"/",
		"Revoke all sessions for current user",
		routes.Func(h.handleDeleteAllSessions),
	)
	sessions.Delete(
		"/{sid}",
		"Revoke a specific session for current user",
		routes.Func(h.handleDeleteSession),
	)
	authRoutes.Include("/sessions", sessions)

	root := routes.NewBlueprint()
	root.Include(opts.BasePath, authRoutes)
	return root.Routes()
}

func (h *Handler) handleLogin(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	defer r.Body.Close()

	var req loginRequest
	if err := decoder.DecodeJSONBody(r, &req); err != nil {
		if errors.Is(err, decoder.ErrBodyTooLarge) {
			response.WriteJSONError(w, stdhttp.StatusRequestEntityTooLarge, "body_too_large", "request body too large")
			return
		}
		response.WriteJSONError(w, stdhttp.StatusBadRequest, "invalid_json", "invalid json")
		return
	}

	sessionOnly, err := loginSessionOnly(req)
	if err != nil {
		response.WriteJSONError(w, stdhttp.StatusBadRequest, "invalid_persistence", "persistence must be session or persistent")
		return
	}

	tokens, ok, err := h.service.Login(r.Context(), req.Username, req.Password, auth.IssueOptions{
		SessionOnly: sessionOnly,
	})
	if err != nil {
		writeAuthFailure(w, err)
		return
	}
	if !ok {
		response.WriteJSONError(w, stdhttp.StatusUnauthorized, "unauthorized", "invalid credentials")
		return
	}

	csrf := uuid.NewString()
	cfg := h.cookieConfig(r, sessionOnly)
	authsession.SetRefreshCookie(w, tokens.Refresh, tokens.RefreshExpiresAt, withNameAndPath(cfg, h.options.RefreshCookieName, h.options.RefreshCookiePath))
	authsession.SetCSRFCookie(w, csrf, tokens.RefreshExpiresAt, withNameAndPath(cfg, h.options.CSRFCookieName, h.options.CSRFCookiePath))

	response.WriteJSON(w, stdhttp.StatusOK, loginResponse{
		AccessToken: tokens.Access,
		ExpiresAt:   tokens.AccessExpiresAt.Format(time.RFC3339),
		CSRFToken:   csrf,
	})
}

func (h *Handler) handleRefresh(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	refresh, ok := refreshTokenFromContext(r.Context())
	if !ok {
		writeAuthMisconfigured(w)
		return
	}

	result, ok, err := h.service.Refresh(r.Context(), refresh)
	if err != nil {
		writeAuthFailure(w, err)
		return
	}
	if !ok {
		response.WriteJSONError(w, stdhttp.StatusUnauthorized, "unauthorized", "refresh token invalid")
		return
	}

	csrf := uuid.NewString()
	cfg := h.cookieConfig(r, result.SessionOnly)
	authsession.SetRefreshCookie(w, result.Refresh, result.RefreshExpiresAt, withNameAndPath(cfg, h.options.RefreshCookieName, h.options.RefreshCookiePath))
	authsession.SetCSRFCookie(w, csrf, result.RefreshExpiresAt, withNameAndPath(cfg, h.options.CSRFCookieName, h.options.CSRFCookiePath))

	response.WriteJSON(w, stdhttp.StatusOK, refreshResponse{
		AccessToken: result.Access,
		ExpiresAt:   result.AccessExpiresAt.Format(time.RFC3339),
		CSRFToken:   csrf,
	})
}

func (h *Handler) handleLogout(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	refresh, ok := refreshTokenFromContext(r.Context())
	if !ok {
		writeAuthMisconfigured(w)
		return
	}

	if err := h.service.Logout(r.Context(), refresh); err != nil && !errors.Is(err, authjwt.ErrUnauthorized) {
		writeAuthFailure(w, err)
		return
	}

	h.clearSessionCookies(w, r)
	w.WriteHeader(stdhttp.StatusNoContent)
}

func (h *Handler) handleDeleteCurrentSession(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	claims, ok := h.authenticatedClaims(w, r)
	if !ok {
		return
	}
	if _, err := h.service.RevokeSession(r.Context(), claims.Subject, claims.SessionID); err != nil {
		writeAuthFailure(w, err)
		return
	}
	h.clearSessionCookies(w, r)
	w.WriteHeader(stdhttp.StatusNoContent)
}

func (h *Handler) handleDeleteAllSessions(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	claims, ok := h.authenticatedClaims(w, r)
	if !ok {
		return
	}
	if err := h.service.RevokeAllSessions(r.Context(), claims.Subject); err != nil {
		writeAuthFailure(w, err)
		return
	}
	h.clearSessionCookies(w, r)
	w.WriteHeader(stdhttp.StatusNoContent)
}

func (h *Handler) handleDeleteSession(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	claims, ok := h.authenticatedClaims(w, r)
	if !ok {
		return
	}
	sessionID := strings.TrimSpace(chi.URLParam(r, "sid"))
	if sessionID == "" {
		response.WriteJSONError(w, stdhttp.StatusBadRequest, "invalid_session", "session id is required")
		return
	}
	if _, err := h.service.RevokeSession(r.Context(), claims.Subject, sessionID); err != nil {
		writeAuthFailure(w, err)
		return
	}
	if sessionID == claims.SessionID {
		h.clearSessionCookies(w, r)
	}
	w.WriteHeader(stdhttp.StatusNoContent)
}

func (h *Handler) authenticatedClaims(w stdhttp.ResponseWriter, r *stdhttp.Request) (authjwt.Claims, bool) {
	if claims, ok := authjwt.ClaimsFromContext(r.Context()); ok {
		return claims, true
	}
	writeAuthMisconfigured(w)
	return authjwt.Claims{}, false
}

func (h *Handler) cookieConfig(r *stdhttp.Request, sessionOnly bool) authsession.CookieConfig {
	cfg := authsession.DefaultCookieConfig(r, authsession.CookieTrustOptions{
		TrustedProxies: h.options.TrustedProxies,
	})
	cfg.SessionOnly = sessionOnly
	return cfg
}

func (h *Handler) clearSessionCookies(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	cfg := h.cookieConfig(r, false)
	authsession.ClearCookie(w, withNameAndPath(cfg, h.options.RefreshCookieName, h.options.RefreshCookiePath))
	authsession.ClearCookie(w, withNameAndPath(cfg, h.options.CSRFCookieName, h.options.CSRFCookiePath))
}

func withNameAndPath(base authsession.CookieConfig, name, path string) authsession.CookieConfig {
	base.Name = name
	base.Path = path
	return base
}

func writeAuthMisconfigured(w stdhttp.ResponseWriter) {
	response.WriteJSONError(w, stdhttp.StatusInternalServerError, errAuthMisconfiguredCode, errAuthMisconfiguredMessage)
}

func loginSessionOnly(req loginRequest) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(req.Persistence)) {
	case "session":
		return true, nil
	case "persistent":
		return false, nil
	default:
		return false, errors.New("invalid persistence")
	}
}
