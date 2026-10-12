package rest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/leksuu/platform/internal/modules/auth/security"
	"github.com/leksuu/platform/internal/shared/authn"
)

type accounts map[string]struct {
	active bool
	roles  []string
}

func (a accounts) AccountState(_ context.Context, id string) (bool, []string, error) {
	s := a[id]
	return s.active, s.roles, nil
}

func TestAuthenticatorChecksCurrentAccountState(t *testing.T) {
	jwt := security.NewJWT([]byte("test-secret"), 15*time.Minute, time.Now)
	a := NewAuthenticator(jwt)
	acc := accounts{"u1": {active: true, roles: []string{"student"}}}
	a.SetAccountChecker(acc)
	token, _, _ := jwt.Issue("u1", []string{"student", "admin"}) // stale admin role in the token

	var got authn.Principal
	h := a.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got, _ = authn.FromContext(r.Context()) }))
	call := func() int {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if code := call(); code != http.StatusOK || got.HasRole("admin") {
		t.Fatalf("active user: code=%d roles=%v (token roles must not win over current ones)", code, got.Roles)
	}
	acc["u1"] = struct {
		active bool
		roles  []string
	}{active: false}
	a.Forget("u1")
	if code := call(); code != http.StatusUnauthorized {
		t.Fatalf("suspended user's existing token: want 401, got %d", code)
	}
	if _, err := a.ParseToken(token); err == nil {
		t.Fatal("websocket token parsing must apply the same check")
	}
}

func TestOptionalAuth(t *testing.T) {
	jwt := security.NewJWT([]byte("test-secret"), 15*time.Minute, time.Now)
	a := NewAuthenticator(jwt)
	var seen bool
	h := a.Optional(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, seen = authn.FromContext(r.Context()) }))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || seen {
		t.Fatalf("anonymous: code=%d principal=%v", rec.Code, seen)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer garbage")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("an invalid token must be refused so the client refreshes it, got %d", rec.Code)
	}
}

// Learning-only APIs admit learner accounts; teacher and admin accounts are
// refused (403), whatever roles their token was issued with.
func TestLearnerGuardAdmitsOnlyLearners(t *testing.T) {
	jwt := security.NewJWT([]byte("test-secret"), 15*time.Minute, time.Now)
	a := NewAuthenticator(jwt)
	a.SetAccountChecker(accounts{
		"learner":   {active: true, roles: []string{"student"}},
		"teacher":   {active: true, roles: []string{"teacher"}},
		"admin":     {active: true, roles: []string{"admin"}},
		"ex-admin":  {active: true, roles: []string{"student"}}, // admin role removed since login
		"admin+stu": {active: true, roles: []string{"student", "admin"}},
	})
	h := LearnerGuard{a}.Require(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	for id, want := range map[string]int{"learner": 200, "teacher": 403, "admin": 403, "ex-admin": 200, "admin+stu": 403} {
		token, _, _ := jwt.Issue(id, []string{"admin"})
		req := httptest.NewRequest(http.MethodGet, "/srs/due", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("%s: want %d, got %d", id, want, rec.Code)
		}
	}
	// No token at all: 401, as before.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/srs/due", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous: want 401, got %d", rec.Code)
	}
}
