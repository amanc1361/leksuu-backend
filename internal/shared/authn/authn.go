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
