package authhttp

import "github.com/Ithildur/EiluneKit/auth"

// ValidateStaticPasswordVisibleASCII validates a static password.
// NewHandler does not call it automatically.
// ValidateStaticPasswordVisibleASCII 校验静态密码。
// NewHandler 不会自动调用它。
func ValidateStaticPasswordVisibleASCII(password string) error {
	return auth.ValidateStaticPasswordVisibleASCII(password)
}

// VerifyCredential compares expected and got with an exact byte match.
// VerifyCredential 使用精确字节匹配比较 expected 和 got。
func VerifyCredential(expected, got string) bool {
	return auth.VerifyCredential(expected, got)
}

// NewStaticPasswordAuthenticator returns a LoginAuthenticator for one fixed password.
// NewStaticPasswordAuthenticator 返回基于固定密码的 LoginAuthenticator。
func NewStaticPasswordAuthenticator(userID, expectedPassword string) (LoginAuthenticator, error) {
	return auth.NewStaticPasswordAuthenticator(userID, expectedPassword)
}
