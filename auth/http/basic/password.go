package basic

import "github.com/Ithildur/EiluneKit/auth"

// ValidateStaticPassword validates a static password.
// NewHandler does not call it automatically.
// ValidateStaticPassword 校验静态密码。
// NewHandler 不会自动调用它。
func ValidateStaticPassword(password string) error {
	return auth.ValidateStaticPassword(password)
}

// VerifyCredential compares expected and got with an exact byte match.
// VerifyCredential 使用精确字节匹配比较 expected 和 got。
func VerifyCredential(expected, got string) bool {
	return auth.VerifyCredential(expected, got)
}

// NewStaticPassword returns a LoginAuthenticator for one fixed password.
// NewStaticPassword 返回基于固定密码的 LoginAuthenticator。
func NewStaticPassword(userID, expectedPassword string) (LoginAuthenticator, error) {
	return auth.NewStaticPassword(userID, expectedPassword)
}
