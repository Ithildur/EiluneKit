package rbachttp

import (
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"strings"
	"time"

	authcore "github.com/Ithildur/EiluneKit/auth"
	corerbac "github.com/Ithildur/EiluneKit/auth/rbac"
	"github.com/Ithildur/EiluneKit/clientip"
	"github.com/Ithildur/EiluneKit/http/decoder"
	"github.com/Ithildur/EiluneKit/http/middleware"
	"github.com/Ithildur/EiluneKit/http/response"
	"github.com/Ithildur/EiluneKit/http/routes"

	"github.com/go-chi/chi/v5"
)

// Handler serves JSON bearer RBAC auth endpoints.
// Handler 提供 JSON bearer RBAC 认证端点。
type Handler struct {
	auth       *corerbac.Service
	options    Options
	middleware Middleware
}

type loginRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	Persistence string `json:"persistence,omitempty"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type logoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type tokenResponse struct {
	AccessToken      string             `json:"access_token"`
	RefreshToken     string             `json:"refresh_token"`
	AccessExpiresAt  string             `json:"access_expires_at"`
	RefreshExpiresAt string             `json:"refresh_expires_at"`
	ExpiresAt        string             `json:"expires_at"`
	SessionOnly      bool               `json:"session_only"`
	User             authcore.Principal `json:"user"`
}

type principalResponse struct {
	User authcore.Principal `json:"user"`
}

// NewHandler returns a Handler.
// NewHandler 返回 Handler。
func NewHandler(service *corerbac.Service, opts Options) (*Handler, error) {
	if service == nil {
		return nil, corerbac.ErrServiceMisconfigured
	}
	effective := applyOptions(DefaultOptions(), opts)
	return &Handler{
		auth:       service,
		options:    effective,
		middleware: NewMiddleware(service, effective.RolePolicy),
	}, nil
}

// Middleware returns route middleware helpers for this Handler.
// Middleware 返回该 Handler 的路由 middleware 辅助工具。
func (h *Handler) Middleware() Middleware {
	if h == nil {
		return Middleware{}
	}
	return h.middleware
}

// Register mounts the auth routes on r.
// Register 在 r 上挂载认证路由。
func (h *Handler) Register(r chi.Router) error {
	if h == nil {
		return fmt.Errorf("rbac auth: handler is nil")
	}
	return routes.Mount(r, "", h.Routes())
}

// Routes returns the auth routes.
// Routes 返回认证路由。
func (h *Handler) Routes() []routes.Route {
	if h == nil {
		panic("rbac auth: handler is nil")
	}
	maxBytes := h.options.MaxBodyBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxBodyBytes
	}

	authRoutes := routes.NewBlueprint(
		routes.DefaultTags("auth"),
		routes.DefaultAuth(routes.AuthPublic),
	)
	authRoutes.Post(
		"/login",
		"Login",
		routes.Func(h.handleLogin),
		routes.Use(middleware.LimitBody(maxBytes)),
		routes.Use(middleware.RequireJSONBody),
	)
	authRoutes.Post(
		"/refresh",
		"Refresh access token",
		routes.Func(h.handleRefresh),
		routes.Use(middleware.LimitBody(maxBytes)),
		routes.Use(middleware.RequireJSONBody),
	)
	authRoutes.Post(
		"/logout",
		"Logout",
		routes.Func(h.handleLogout),
		routes.Use(middleware.LimitBody(maxBytes)),
		routes.Use(middleware.RequireJSONBody),
	)
	authRoutes.Get(
		"/me",
		"Current user",
		routes.Func(h.handleMe),
		routes.Auth(routes.AuthRequired),
		routes.Use(h.middleware.RequireAuth()),
	)

	root := routes.NewBlueprint()
	root.Include(h.options.BasePath, authRoutes)
	return root.Routes()
}

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var req loginRequest
	if err := decoder.DecodeJSONBody(r, &req); err != nil {
		writeDecodeError(w, err)
		return
	}
	sessionOnly, err := loginSessionOnly(req.Persistence)
	if err != nil {
		response.WriteJSONError(w, http.StatusBadRequest, "invalid_persistence", "persistence must be session or persistent")
		return
	}
	tokens, ok, err := h.auth.Login(r.Context(), corerbac.LoginRequest{
		Username:    req.Username,
		Password:    req.Password,
		SessionOnly: sessionOnly,
		LockoutKey:  h.lockoutKey(r, req.Username),
	})
	if err != nil {
		writeAuthFailure(w, err)
		return
	}
	if !ok {
		response.WriteJSONError(w, http.StatusUnauthorized, "unauthorized", "invalid credentials")
		return
	}
	response.WriteJSON(w, http.StatusOK, responseFromTokens(tokens))
}

func (h *Handler) handleRefresh(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var req refreshRequest
	if err := decoder.DecodeJSONBody(r, &req); err != nil {
		writeDecodeError(w, err)
		return
	}
	result, ok, err := h.auth.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		if errors.Is(err, corerbac.ErrRefreshTokenRequired) {
			response.WriteJSONError(w, http.StatusBadRequest, "missing_refresh_token", "refresh token is required")
			return
		}
		writeAuthFailure(w, err)
		return
	}
	if !ok {
		response.WriteJSONError(w, http.StatusUnauthorized, "unauthorized", "refresh token invalid")
		return
	}
	response.WriteJSON(w, http.StatusOK, responseFromRefresh(result))
}

func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var req logoutRequest
	if err := decoder.DecodeJSONBody(r, &req); err != nil {
		writeDecodeError(w, err)
		return
	}
	if err := h.auth.Logout(r.Context(), req.RefreshToken); err != nil {
		if errors.Is(err, corerbac.ErrRefreshTokenRequired) {
			response.WriteJSONError(w, http.StatusBadRequest, "missing_refresh_token", "refresh token is required")
			return
		}
		writeAuthFailure(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleMe(w http.ResponseWriter, r *http.Request) {
	principal, ok := authcore.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteJSONError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	response.WriteJSON(w, http.StatusOK, principalResponse{User: principal})
}

func (h *Handler) lockoutKey(r *http.Request, username string) string {
	ip, ok := clientip.FromRequest(r, clientip.Options{
		TrustedProxies: append([]netip.Prefix(nil), h.options.TrustedProxies...),
	})
	ipPart := "unknown"
	if ok {
		ipPart = ip.String()
	}
	return "ip:" + ipPart + "|username:" + strings.TrimSpace(username)
}

func loginSessionOnly(persistence string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(persistence)) {
	case "", "persistent":
		return false, nil
	case "session":
		return true, nil
	default:
		return false, fmt.Errorf("invalid persistence")
	}
}

func responseFromTokens(tokens corerbac.Tokens) tokenResponse {
	return tokenResponse{
		AccessToken:      tokens.AccessToken,
		RefreshToken:     tokens.RefreshToken,
		AccessExpiresAt:  formatTime(tokens.AccessExpiresAt),
		RefreshExpiresAt: formatTime(tokens.RefreshExpiresAt),
		ExpiresAt:        formatTime(tokens.AccessExpiresAt),
		SessionOnly:      tokens.SessionOnly,
		User:             tokens.Principal,
	}
}

func responseFromRefresh(result corerbac.RefreshResult) tokenResponse {
	return tokenResponse{
		AccessToken:      result.AccessToken,
		RefreshToken:     result.RefreshToken,
		AccessExpiresAt:  formatTime(result.AccessExpiresAt),
		RefreshExpiresAt: formatTime(result.RefreshExpiresAt),
		ExpiresAt:        formatTime(result.AccessExpiresAt),
		SessionOnly:      result.SessionOnly,
		User:             result.Principal,
	}
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func writeDecodeError(w http.ResponseWriter, err error) {
	if errors.Is(err, decoder.ErrBodyTooLarge) {
		response.WriteJSONError(w, http.StatusRequestEntityTooLarge, "body_too_large", "request body too large")
		return
	}
	response.WriteJSONError(w, http.StatusBadRequest, "invalid_json", "invalid json")
}
