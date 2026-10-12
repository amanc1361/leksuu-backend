// Package authn carries the authenticated principal across the request context.
// It is shared kernel: the auth module writes the principal after validating a
// token, and ANY module reads it without depending on auth's internals.
package authn

import "context"

// Principal is the authenticated caller.
type Principal struct {
	Subject string // user id
	Roles   []string
}

func (p Principal) HasRole(role string) bool {
	for _, r := range p.Roles {
		if r == role {
			return true
		}
	}
	return false
}

type ctxKey int

const principalKey ctxKey = 0

func NewContext(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey).(Principal)
	return p, ok
}

// Account types. Every account is exactly one of these: a learner uses the
// learning app, a teacher only the teaching portal, an admin only the admin
// panel. The type follows the account's roles (admin wins over teacher).
const (
	AccountLearner = "learner"
	AccountTeacher = "teacher"
	AccountAdmin   = "admin"
)

// AccountType reports which kind of account the principal is.
func (p Principal) AccountType() string {
	switch {
	case p.HasRole("admin"):
		return AccountAdmin
	case p.HasRole("teacher"):
		return AccountTeacher
	default:
		return AccountLearner
	}
}
