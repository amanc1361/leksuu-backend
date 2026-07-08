// Package security provides cryptographic adapters that satisfy the auth
// usecase ports. Standard-library only, verifiable against published vectors.
package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	pbkdf2Iterations = 210_000 // OWASP 2023 guidance, PBKDF2-HMAC-SHA256
	pbkdf2KeyLen     = 32
	pbkdf2SaltLen    = 16
	pbkdf2Scheme     = "pbkdf2_sha256"
)

// PasswordHasher implements usecase.Hasher. Encoded form:
// pbkdf2_sha256$<iter>$<saltB64>$<hashB64>. The scheme prefix makes a future
// swap to argon2id a drop-in.
type PasswordHasher struct{}

func NewPasswordHasher() *PasswordHasher { return &PasswordHasher{} }

func (PasswordHasher) Hash(plain string) (string, error) {
	if plain == "" {
		return "", errors.New("empty password")
	}
	salt := make([]byte, pbkdf2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	dk := pbkdf2([]byte(plain), salt, pbkdf2Iterations, pbkdf2KeyLen)
	return fmt.Sprintf("%s$%d$%s$%s", pbkdf2Scheme, pbkdf2Iterations,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(dk)), nil
}

func (PasswordHasher) Verify(encoded, plain string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != pbkdf2Scheme {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 1 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	got := pbkdf2([]byte(plain), salt, iter, len(want))
	return subtle.ConstantTimeCompare(got, want) == 1
}

func pbkdf2(password, salt []byte, iter, keyLen int) []byte {
	prf := hmac.New(sha256.New, password)
	hashLen := prf.Size()
	numBlocks := (keyLen + hashLen - 1) / hashLen
	dk := make([]byte, 0, numBlocks*hashLen)
	var block [4]byte
	T := make([]byte, hashLen)
	U := make([]byte, hashLen)
	for b := 1; b <= numBlocks; b++ {
		binary.BigEndian.PutUint32(block[:], uint32(b))
		prf.Reset()
		prf.Write(salt)
		prf.Write(block[:])
		T = prf.Sum(T[:0])
		copy(U, T)
		for n := 2; n <= iter; n++ {
			prf.Reset()
			prf.Write(U)
			U = prf.Sum(U[:0])
			for x := range T {
				T[x] ^= U[x]
			}
		}
		dk = append(dk, T...)
	}
	return dk[:keyLen]
}
