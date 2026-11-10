package authhttp

import (
	"net/http"

	authjwt "github.com/Ithildur/EiluneKit/auth/jwt"
	"github.com/Ithildur/EiluneKit/http/routes"
	"github.com/Ithildur/EiluneKit/internal/passkeyhttp"
)

func (h *Handler) passkeyRoutes(rate *RateLimitOptions) []routes.Route {
	transport := passkeyhttp.Handler{
		Service: h.options.Passkeys, Authenticate: h.bearer,
		UserID: func(r *http.Request) (string, bool) {
			claims, ok := authjwt.ClaimsFromContext(r.Context())
			return claims.Subject, ok
		},
		Login: h.handlePasskeyLogin, Failure: writeAuthFailure,
	}
	result := transport.Routes(h.options.MaxBodyBytes, LoginRateLimit(rate))
	for i := range result {
		result[i].OperationID = "auth" + result[i].OperationID
	}
	return result
}

func (h *Handler) handlePasskeyLogin(w http.ResponseWriter, r *http.Request, flowID string, credential []byte) {
	tokens, sessionOnly, err := h.service.LoginPasskey(r.Context(), h.options.Passkeys, flowID, credential)
	if err != nil {
		if !passkeyhttp.WriteError(w, err) {
			writeAuthFailure(w, err)
		}
		return
	}
	h.completeLogin(w, r, tokens, "", sessionOnly)
}
