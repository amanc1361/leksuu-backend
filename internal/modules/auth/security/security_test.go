package security

import (
	"encoding/hex"
	"testing"
	"time"
)

func TestPBKDF2Vectors(t *testing.T) {
	cases := []struct {
		iter int
		want string
	}{
		{1, "120fb6cffcf8b32c43e7225256c4f837a86548c92ccc35480805987cb70be17b"},
		{2, "ae4d0c95af6b46d32d0adff928f06dd02a303f8ef3c251dfd6e2d85a95474c43"},
	}
	for _, c := range cases {
		got := hex.EncodeToString(pbkdf2([]byte("password"), []byte("salt"), c.iter, 32))
		if got != c.want {
			t.Fatalf("iter=%d got=%s want=%s", c.iter, got, c.want)
		}
	}
}

func TestPasswordHashVerify(t *testing.T) {
	h := NewPasswordHasher()
	enc, err := h.Hash("s3cret")
	if err != nil {
		t.Fatal(err)
	}
	if !h.Verify(enc, "s3cret") {
		t.Fatal("correct password rejected")
	}
	if h.Verify(enc, "nope") {
		t.Fatal("wrong password accepted")
	}
	if enc2, _ := h.Hash("s3cret"); enc == enc2 {
		t.Fatal("salt not randomized")
	}
}

func TestJWTIssueParse(t *testing.T) {
	j := NewJWT([]byte("secret"), time.Minute, time.Now)
	tok, ttl, err := j.Issue("u1", []string{"student"})
	if err != nil || ttl != 60 {
		t.Fatalf("issue: ttl=%d err=%v", ttl, err)
	}
	claims, err := j.Parse(tok)
	if err != nil || claims.Subject != "u1" || len(claims.Roles) != 1 {
		t.Fatalf("parse: %+v err=%v", claims, err)
	}
	if _, err := NewJWT([]byte("other"), time.Minute, time.Now).Parse(tok); err == nil {
		t.Fatal("forged token accepted")
	}
	exp := NewJWT([]byte("secret"), -time.Second, time.Now)
	t2, _, _ := exp.Issue("u1", nil)
	if _, err := exp.Parse(t2); err == nil {
		t.Fatal("expired token accepted")
	}
}

func TestRefreshFactory(t *testing.T) {
	f := NewRefreshFactory()
	raw, hash, err := f.New()
	if err != nil {
		t.Fatal(err)
	}
	if raw == hash || f.Hash(raw) != hash {
		t.Fatal("refresh hashing inconsistent")
	}
}
