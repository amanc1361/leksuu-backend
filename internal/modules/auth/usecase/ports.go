// Package usecase contains the auth application business rules. It depends only
// on the module's domain and the ports declared here; concrete implementations
// (security, repo, rest) are injected by the module facade.
package usecase

import "time"

// Claims is the authenticated principal carried by an access token.
type Claims struct {
	Subject string
	Roles   []string
}

// Hasher hashes/verifies passwords (impl: security, PBKDF2).
type Hasher interface {
	Hash(plain string) (string, error)
	Verify(encoded, plain string) bool
}

// AccessTokenIssuer mints short-lived access tokens (token + lifetime seconds).
type AccessTokenIssuer interface {
	Issue(subject string, roles []string) (token string, expiresIn int64, err error)
}

// AccessTokenParser validates an access token and extracts its claims.
type AccessTokenParser interface {
	Parse(token string) (Claims, error)
}

// RefreshTokenFactory creates opaque refresh tokens and hashes them for storage.
type RefreshTokenFactory interface {
	New() (raw string, hash string, err error)
	Hash(raw string) string
}

// Clock is injectable so time-dependent logic stays testable.
type Clock func() time.Time
