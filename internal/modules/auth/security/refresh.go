package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// RefreshFactory implements usecase.RefreshTokenFactory. Tokens are opaque
// random strings; only their SHA-256 hash is persisted.
type RefreshFactory struct{}

func NewRefreshFactory() *RefreshFactory { return &RefreshFactory{} }

func (RefreshFactory) New() (raw string, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	return raw, RefreshFactory{}.Hash(raw), nil
}

func (RefreshFactory) Hash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
