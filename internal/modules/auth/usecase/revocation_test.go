package usecase_test

import (
	"context"
	"testing"

	"github.com/leksuu/platform/internal/modules/auth/domain"
	"github.com/leksuu/platform/internal/modules/auth/usecase"
)

func TestRegisterRejectsInvalidEmail(t *testing.T) {
	ctx := context.Background()
	s := newService()
	for _, e := range []string{"not-an-email", "a@b", "@b.com", "a@.com", "a b@c.com", "Name <a@b.com>", "a@b.com."} {
		if _, err := s.Register(ctx, usecase.RegisterInput{Email: e, Password: "pw"}); err != domain.ErrEmailInvalid {
			t.Fatalf("%q: want ErrEmailInvalid, got %v", e, err)
		}
	}
	if _, err := s.Register(ctx, usecase.RegisterInput{Email: " Sara.K+test@Example.co ", Password: "pw"}); err != nil {
		t.Fatalf("a valid address must register: %v", err)
	}
}

// A suspended account can neither refresh its session nor pass the account
// check that the request authenticator runs; reactivation restores access.
func TestSuspensionRevokesSessions(t *testing.T) {
	ctx := context.Background()
	s := newService()
	u, _ := s.Register(ctx, usecase.RegisterInput{Email: "s@x.com", Password: "pw", DisplayName: "S"})
	pair, err := s.Login(ctx, "s@x.com", "pw", "d1")
	if err != nil {
		t.Fatal(err)
	}
	var changed []string
	s.OnAccountChange(func(id string) { changed = append(changed, id) })

	if err := s.SetUserStatus(ctx, u.ID, domain.StatusSuspended); err != nil {
		t.Fatal(err)
	}
	if len(changed) != 1 || changed[0] != u.ID {
		t.Fatalf("status change must notify the authenticator, got %v", changed)
	}
	if active, _, err := s.AccountState(ctx, u.ID); err != nil || active {
		t.Fatalf("suspended account must be inactive: active=%v err=%v", active, err)
	}
	if _, err := s.Refresh(ctx, pair.RefreshToken, "d1"); err != domain.ErrTokenInvalid {
		t.Fatalf("suspended refresh: want ErrTokenInvalid, got %v", err)
	}
	_ = s.SetUserStatus(ctx, u.ID, domain.StatusActive)
	// The family was revoked: the old refresh token stays dead, a new login works.
	if _, err := s.Refresh(ctx, pair.RefreshToken, "d1"); err == nil {
		t.Fatal("a refresh token revoked at suspension must not come back")
	}
	if _, err := s.Login(ctx, "s@x.com", "pw", "d1"); err != nil {
		t.Fatalf("reactivated login: %v", err)
	}
	if active, roles, _ := s.AccountState(ctx, u.ID); !active || len(roles) == 0 {
		t.Fatalf("reactivated: active=%v roles=%v", active, roles)
	}
}
