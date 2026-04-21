package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"strings"
)

var (
	// ErrStaticPasswordUserIDEmpty reports an empty user ID for static password auth.
	// ErrStaticPasswordUserIDEmpty 表示固定密码认证使用了空 user ID。
	ErrStaticPasswordUserIDEmpty = errors.New("static password user id is required")
	// ErrPasswordEmpty reports an empty password.
	// ErrPasswordEmpty 表示密码为空。
	ErrPasswordEmpty = errors.New("password is empty")
	// ErrPasswordContainsSpace reports whitespace in a static password.
	// ErrPasswordContainsSpace 表示固定密码中包含空白字符。
	ErrPasswordContainsSpace = errors.New("password must not contain whitespace")
	// ErrPasswordInvalidCharacter reports a character outside visible ASCII.
	// ErrPasswordInvalidCharacter 表示存在可见 ASCII 之外的字符。
	ErrPasswordInvalidCharacter = errors.New("password must contain only ASCII letters, digits, and common symbols")
)

// ValidateStaticPasswordVisibleASCII validates a static password using visible ASCII only.
// ValidateStaticPasswordVisibleASCII 使用仅可见 ASCII 规则校验静态密码。
func ValidateStaticPasswordVisibleASCII(password string) error {
	return validateVisibleASCIIWithoutWhitespace(password, ErrPasswordEmpty, ErrPasswordContainsSpace, ErrPasswordInvalidCharacter)
}

// VerifyCredential compares two credential strings using an exact byte match.
// VerifyCredential 使用精确字节匹配比较两段凭据。
func VerifyCredential(expected, got string) bool {
	if len(expected) != len(got) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(got)) == 1
}

// NewStaticPasswordAuthenticator builds a LoginAuthenticator backed by one fixed password.
// NewStaticPasswordAuthenticator 构造一个使用固定密码的 LoginAuthenticator。
func NewStaticPasswordAuthenticator(userID, expectedPassword string) (LoginAuthenticator, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, ErrStaticPasswordUserIDEmpty
	}
	if expectedPassword == "" {
		return nil, ErrPasswordEmpty
	}
	principal := strings.TrimSpace(userID)
	return LoginAuthenticatorFunc(func(ctx context.Context, username, password string) (string, bool, error) {
		if !VerifyCredential(expectedPassword, password) {
			return "", false, nil
		}
		return principal, true, nil
	}), nil
}

func validateVisibleASCIIWithoutWhitespace(value string, emptyErr, whitespaceErr, invalidErr error) error {
	if value == "" {
		return emptyErr
	}
	for _, r := range value {
		switch {
		case r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\v' || r == '\f':
			return whitespaceErr
		case r < '!' || r > '~':
			return invalidErr
		}
	}
	return nil
}
