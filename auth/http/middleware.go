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

// ErrAccessTokenValidatorMissing means a Bearer middleware was built without a validator.
// ErrAccessTokenValidatorMissing 表示 Bearer 中间件缺少 token validator。
var ErrAccessTokenValidatorMissing = errors.New("access token validator is required")

// ErrAPIKeyValidatorMissing means an API-key middleware was built without a validator.
// ErrAPIKeyValidatorMissing 表示 API-key 中间件缺少 validator。
var ErrAPIKeyValidatorMissing = errors.New("api key validator is required")

const defaultAPIKeyHeader = "X-API-Key"

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
			req, ok := requestWithBearerClaims(w, r, auth, r.Header.Get("Authorization"))
			if !ok {
				return
			}

			next.ServeHTTP(w, req)
		})
	}, nil
}

// OptionalBearer returns middleware that attaches claims when a Bearer token is present.
// Missing Authorization is accepted; malformed or invalid tokens are rejected.
// OptionalBearer 返回 Bearer token 可选中间件。
// 未提供 Authorization 会继续执行；格式错误或 token 无效会被拒绝。
func OptionalBearer(auth AccessTokenValidator) (func(stdhttp.Handler) stdhttp.Handler, error) {
	if auth == nil {
		return nil, ErrAccessTokenValidatorMissing
	}

	return func(next stdhttp.Handler) stdhttp.Handler {
		return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
			header := strings.TrimSpace(r.Header.Get("Authorization"))
			if header == "" {
				next.ServeHTTP(w, r)
				return
			}

			req, ok := requestWithBearerClaims(w, r, auth, header)
			if !ok {
				return
			}

			next.ServeHTTP(w, req)
		})
	}, nil
}

func requestWithBearerClaims(w stdhttp.ResponseWriter, r *stdhttp.Request, auth AccessTokenValidator, header string) (*stdhttp.Request, bool) {
	token, ok := parseBearerHeader(header)
	if !ok {
		response.WriteJSONError(w, stdhttp.StatusUnauthorized, "unauthorized", "missing or invalid bearer token")
		return nil, false
	}

	claims, ok, err := auth.ValidateAccessToken(r.Context(), token)
	switch {
	case err != nil:
		writeAuthFailure(w, err)
		return nil, false
	case !ok:
		response.WriteJSONError(w, stdhttp.StatusUnauthorized, "unauthorized", "token invalid or expired")
		return nil, false
	}

	return r.WithContext(authjwt.WithClaims(r.Context(), claims)), true
}

// RequireAPIKey returns middleware that validates an API key from header.
// Empty header uses X-API-Key.
// RequireAPIKey 返回从 header 校验 API key 的中间件。
// header 为空时使用 X-API-Key。
func RequireAPIKey(auth APIKeyValidator, header string) (func(stdhttp.Handler) stdhttp.Handler, error) {
	if auth == nil {
		return nil, ErrAPIKeyValidatorMissing
	}

	header = strings.TrimSpace(header)
	if header == "" {
		header = defaultAPIKeyHeader
	}

	return func(next stdhttp.Handler) stdhttp.Handler {
		return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
			key := strings.TrimSpace(r.Header.Get(header))
			if key == "" {
				response.WriteJSONError(w, stdhttp.StatusUnauthorized, "unauthorized", "missing api key")
				return
			}

			ok, err := auth.ValidateAPIKey(r.Context(), key)
			switch {
			case err != nil:
				writeAuthFailure(w, err)
				return
			case !ok:
				response.WriteJSONError(w, stdhttp.StatusUnauthorized, "unauthorized", "api key invalid")
				return
			}

			next.ServeHTTP(w, r)
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
