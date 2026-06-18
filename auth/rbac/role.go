package rbac

// RolePolicy decides whether actual satisfies required.
// RolePolicy 判断 actual 是否满足 required。
type RolePolicy interface {
	Allows(actual string, required string) bool
}

// RoleAllows adapts a function to RolePolicy.
// RoleAllows 将函数适配为 RolePolicy。
type RoleAllows func(actual string, required string) bool

// Allows calls f(actual, required).
// Allows 调用 f(actual, required)。
func (f RoleAllows) Allows(actual string, required string) bool {
	if f == nil {
		return actual == required
	}
	return f(actual, required)
}

// ExactRolePolicy allows only exact role matches.
// ExactRolePolicy 只允许角色精确匹配。
type ExactRolePolicy struct{}

// Allows reports whether actual equals required.
// Allows 返回 actual 是否等于 required。
func (ExactRolePolicy) Allows(actual string, required string) bool {
	return actual == required
}
