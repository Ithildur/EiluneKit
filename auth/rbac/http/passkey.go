package rbachttp

import (
	"net/http"

	authcore "github.com/Ithildur/EiluneKit/auth"
	authhttp "github.com/Ithildur/EiluneKit/auth/http"
	"github.com/Ithildur/EiluneKit/http/response"
	"github.com/Ithildur/EiluneKit/http/routes"
	"github.com/Ithildur/EiluneKit/internal/passkeyhttp"
)

func (h *Handler) passkeyRoutes() []routes.Route {
	var rate routes.Middleware
	if h.options.RateLimit != nil {
		opts := *h.options.RateLimit
		if len(opts.TrustedProxies) == 0 {
			opts.TrustedProxies = h.options.TrustedProxies
		}
		if opts.ClientIPHeaders == nil {
			opts.ClientIPHeaders = h.options.ClientIPHeaders
		}
		rate = authhttp.LoginRateLimit(&opts)
	}
	transport := passkeyhttp.Handler{
		Service: h.options.Passkeys, Authenticate: h.middleware.RequireAuth(),
		UserID: func(r *http.Request) (string, bool) {
			p, ok := authcore.PrincipalFromContext(r.Context())
			return p.Subject, ok && p.Kind == authcore.PrincipalKindUser
		},
		Login: h.handlePasskeyLogin, Failure: writeAuthFailure,
	}
	result := transport.Routes(h.options.MaxBodyBytes, rate)
	for i := range result {
		result[i].OperationID = "rbac" + result[i].OperationID
	}
	return result
}

func (h *Handler) handlePasskeyLogin(w http.ResponseWriter, r *http.Request, flowID string, credential []byte) {
	tokens, ok, err := h.auth.LoginPasskey(r.Context(), h.options.Passkeys, flowID, credential)
	if err != nil {
		if !passkeyhttp.WriteError(w, err) {
			writeAuthFailure(w, err)
		}
		return
	}
	if !ok {
		response.WriteJSONError(w, 401, "unauthorized", "invalid credentials")
		return
	}
	response.WriteJSON(w, 200, responseFromTokens(tokens))
}
