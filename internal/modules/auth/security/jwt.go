package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/leksuu/platform/internal/modules/auth/usecase"
)

var errBadToken = errors.New("invalid token")

// JWT implements usecase.AccessTokenIssuer and usecase.AccessTokenParser (HS256).
type JWT struct {
	secret    []byte
	accessTTL time.Duration
	now       func() time.Time
}

func NewJWT(secret []byte, accessTTL time.Duration, now func() time.Time) *JWT {
	return &JWT{secret: secret, accessTTL: accessTTL, now: now}
}

type jwtClaims struct {
	Sub   string   `json:"sub"`
	Roles []string `json:"roles"`
	Iat   int64    `json:"iat"`
	Exp   int64    `json:"exp"`
}

var jwtHeader = base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

func (j *JWT) sign(input string) string {
	mac := hmac.New(sha256.New, j.secret)
	mac.Write([]byte(input))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (j *JWT) Issue(subject string, roles []string) (string, int64, error) {
	now := j.now()
	payload, err := json.Marshal(jwtClaims{
		Sub: subject, Roles: roles, Iat: now.Unix(), Exp: now.Add(j.accessTTL).Unix(),
	})
	if err != nil {
		return "", 0, err
	}
	input := jwtHeader + "." + base64.RawURLEncoding.EncodeToString(payload)
	return input + "." + j.sign(input), int64(j.accessTTL.Seconds()), nil
}

func (j *JWT) Parse(token string) (usecase.Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return usecase.Claims{}, errBadToken
	}
	input := parts[0] + "." + parts[1]
	if !hmac.Equal([]byte(j.sign(input)), []byte(parts[2])) {
		return usecase.Claims{}, errBadToken
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return usecase.Claims{}, errBadToken
	}
	var c jwtClaims
	if err := json.Unmarshal(raw, &c); err != nil {
		return usecase.Claims{}, errBadToken
	}
	if j.now().Unix() >= c.Exp {
		return usecase.Claims{}, errBadToken
	}
	return usecase.Claims{Subject: c.Sub, Roles: c.Roles}, nil
}
