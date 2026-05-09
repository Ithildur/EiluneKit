package authhttp

import (
	"errors"
	stdhttp "net/http"

	authcore "github.com/Ithildur/EiluneKit/auth"
	authjwt "github.com/Ithildur/EiluneKit/auth/jwt"
	"github.com/Ithildur/EiluneKit/http/response"
)

const errAuthErrorCode = "auth_error"
const errAuthErrorMessage = "auth failed"

func writeAuthFailure(w stdhttp.ResponseWriter, err error) {
	switch {
	case err == nil:
		response.WriteJSONError(w, stdhttp.StatusInternalServerError, errAuthErrorCode, errAuthErrorMessage)
	case errors.Is(err, authjwt.ErrStoreUnavailable):
		response.WriteJSONError(w, stdhttp.StatusServiceUnavailable, "auth_unavailable", "auth is unavailable")
	case isAuthMisconfigured(err):
		writeAuthMisconfigured(w)
	default:
		response.WriteJSONError(w, stdhttp.StatusInternalServerError, errAuthErrorCode, errAuthErrorMessage)
	}
}

func isAuthMisconfigured(err error) bool {
	return errors.Is(err, authcore.ErrServiceMisconfigured) ||
		errors.Is(err, ErrAccessTokenValidatorMissing) ||
		errors.Is(err, ErrAPIKeyValidatorMissing) ||
		errors.Is(err, authcore.ErrTokenManagerMissing) ||
		errors.Is(err, authcore.ErrLoginAuthenticatorMissing) ||
		errors.Is(err, authcore.ErrUserIDEmpty) ||
		errors.Is(err, authjwt.ErrManagerMisconfigured) ||
		errors.Is(err, authjwt.ErrUserIDRequired) ||
		errors.Is(err, authjwt.ErrSessionIDRequired)
}
