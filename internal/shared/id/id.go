// Package id generates random opaque identifiers (128-bit, hex-encoded).
package id

import (
	"crypto/rand"
	"encoding/hex"
)

// New returns a random 128-bit identifier.
func New() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
