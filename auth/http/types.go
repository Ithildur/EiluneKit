package authhttp

import "github.com/Ithildur/EiluneKit/auth"

// AccessTokenValidator validates access tokens for RequireBearer.
// AccessTokenValidator 为 RequireBearer 校验 access token。
type AccessTokenValidator = auth.AccessTokenValidator

// TokenManager provides the auth operations used by Handler.
// TokenManager 为 Handler 提供认证操作。
type TokenManager = auth.TokenManager

// LoginAuthenticator validates login credentials.
// Implementations may ignore username.
// LoginAuthenticator 校验登录凭据。
// 实现可以忽略 username。
type LoginAuthenticator = auth.LoginAuthenticator

// LoginAuthenticatorFunc adapts a function to LoginAuthenticator.
// LoginAuthenticatorFunc 将函数适配为 LoginAuthenticator。
type LoginAuthenticatorFunc = auth.LoginAuthenticatorFunc

type loginRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	Persistence string `json:"persistence,omitempty"`
}

type loginResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresAt   string `json:"expires_at"`
	CSRFToken   string `json:"csrf_token"`
}

type refreshResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresAt   string `json:"expires_at"`
	CSRFToken   string `json:"csrf_token"`
}
