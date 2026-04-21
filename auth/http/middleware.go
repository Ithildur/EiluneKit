package authhttp

import (
	"context"
	"errors"
	stdhttp "net/http"
	"strings"

	authjwt "github.com/Ithildur/EiluneKit/auth/jwt"
	authsession "github.com/Ithildur/EiluneKit/auth/session"
	"github.com/Ithildur/EiluneKit/http/response"
)

var ErrAccessTokenValidatorMissing = errors.New("access token validator is required")

type refreshTokenContextKey struct{}

// RequireBearer returns a Bearer-token middleware.
// Call bearer, err := RequireBearer(auth) and then r.Use(bearer).
// RequireBearer 返回 Bearer token 中间件。
// 调用 bearer, err := RequireBearer(auth)，再执行 r.Use(bearer)。
// Example / 示例:
//
//	bearer, err := authhttp.RequireBearer(jwtManager)
//	if err != nil { ... }
//	r.Use(bearer)
func RequireBearer(auth AccessTokenValidator) (func(stdhttp.Handler) stdhttp.Handler, error) {
	if auth == nil {
		return nil, ErrAccessTokenValidatorMissing
	}

	return func(next stdhttp.Handler) stdhttp.Handler {
		return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
			token, ok := parseBearerHeader(r.Header.Get("Authorization"))
			if !ok {
				response.WriteJSONError(w, stdhttp.StatusUnauthorized, "unauthorized", "missing or invalid bearer token")
				return
			}

			claims, ok, err := auth.ValidateAccessToken(r.Context(), token)
			switch {
			case err != nil:
				writeAuthFailure(w, err)
				return
			case !ok:
				response.WriteJSONError(w, stdhttp.StatusUnauthorized, "unauthorized", "token invalid or expired")
				return
			}

			next.ServeHTTP(w, r.WithContext(authjwt.WithClaims(r.Context(), claims)))
		})
	}, nil
}

func (h *Handler) requireRefreshCookie() func(stdhttp.Handler) stdhttp.Handler {
	return func(next stdhttp.Handler) stdhttp.Handler {
		return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
			if !authsession.ValidateDoubleSubmit(r, h.options.CSRFCookieName, h.options.CSRFHeaderName) {
				response.WriteJSONError(w, stdhttp.StatusUnauthorized, "unauthorized", "csrf validation failed")
				return
			}

			refresh := authsession.ReadCookie(r, h.options.RefreshCookieName)
			if refresh == "" {
				response.WriteJSONError(w, stdhttp.StatusUnauthorized, "unauthorized", "refresh token missing")
				return
			}

			ctx := context.WithValue(r.Context(), refreshTokenContextKey{}, refresh)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func refreshTokenFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	refresh, ok := ctx.Value(refreshTokenContextKey{}).(string)
	if !ok || refresh == "" {
		return "", false
	}
	return refresh, true
}

func parseBearerHeader(header string) (string, bool) {
	parts := strings.Fields(strings.TrimSpace(header))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}
	if parts[1] == "" {
		return "", false
	}
	return parts[1], true
}
