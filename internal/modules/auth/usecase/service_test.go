package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/leksuu/platform/internal/modules/auth/domain"
	"github.com/leksuu/platform/internal/modules/auth/repo"
	"github.com/leksuu/platform/internal/modules/auth/security"
	"github.com/leksuu/platform/internal/modules/auth/usecase"
)

func newService() *usecase.Service {
	return usecase.NewService(
		repo.NewMemoryUserRepo(), repo.NewMemoryTokenRepo(),
		security.NewPasswordHasher(),
		security.NewJWT([]byte("test"), 15*time.Minute, time.Now),
		security.NewRefreshFactory(),
		time.Now,
		usecase.Config{RefreshTTL: 24 * time.Hour},
		nil,
	)
}

func TestRegisterValidationAndDuplicate(t *testing.T) {
	ctx := context.Background()
	s := newService()
	if _, err := s.Register(ctx, usecase.RegisterInput{Email: "", Password: "x"}); err != domain.ErrEmailRequired {
		t.Fatalf("want ErrEmailRequired, got %v", err)
	}
	if _, err := s.Register(ctx, usecase.RegisterInput{Email: "a@b.com", Password: ""}); err != domain.ErrPasswordRequired {
		t.Fatalf("want ErrPasswordRequired, got %v", err)
	}
	u, err := s.Register(ctx, usecase.RegisterInput{Email: "A@B.com", Password: "pw", DisplayName: "A"})
	if err != nil || u.PasswordHash != "" {
		t.Fatalf("register failed or leaked hash: %v", err)
	}
	if _, err := s.Register(ctx, usecase.RegisterInput{Email: "a@b.com", Password: "pw"}); err != domain.ErrEmailTaken {
		t.Fatalf("want ErrEmailTaken, got %v", err)
	}
}

func TestLoginAndRefreshReuse(t *testing.T) {
	ctx := context.Background()
	s := newService()
	_, _ = s.Register(ctx, usecase.RegisterInput{Email: "u@x.com", Password: "pw", DisplayName: "U"})

	if _, err := s.Login(ctx, "u@x.com", "bad", "d1"); err != domain.ErrInvalidCredentials {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
	pair, err := s.Login(ctx, "u@x.com", "pw", "d1")
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := s.Refresh(ctx, pair.RefreshToken, "d1")
	if err != nil || rotated.RefreshToken == pair.RefreshToken {
		t.Fatalf("rotation failed: %v", err)
	}
	if _, err := s.Refresh(ctx, pair.RefreshToken, "d1"); err != domain.ErrTokenReuse {
		t.Fatalf("want ErrTokenReuse, got %v", err)
	}
	if _, err := s.Refresh(ctx, rotated.RefreshToken, "d1"); err == nil {
		t.Fatal("family should be revoked after reuse")
	}
}

func TestAdminUserManagement(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	alice, err := svc.Register(ctx, usecase.RegisterInput{Email: "alice@x.com", Password: "pass1234", DisplayName: "Alice"})
	if err != nil {
		t.Fatal(err)
	}
	bob, err := svc.Register(ctx, usecase.RegisterInput{Email: "bob@x.com", Password: "pass1234", DisplayName: "Bob"})
	if err != nil {
		t.Fatal(err)
	}

	// Baseline list.
	users, total, err := svc.ListUsers(ctx, "", "", "", "", 20, 0)
	if err != nil || total != 2 || len(users) != 2 {
		t.Fatalf("list: total=%d len=%d err=%v", total, len(users), err)
	}

	// Search narrows by email/name.
	got, total, _ := svc.ListUsers(ctx, "alice", "", "", "", 20, 0)
	if total != 1 || got[0].ID != alice.ID {
		t.Fatalf("search alice: total=%d", total)
	}

	// Assign + filter by role.
	if err := svc.SetUserRole(ctx, bob.ID, "teacher", true); err != nil {
		t.Fatal(err)
	}
	teachers, total, _ := svc.ListUsers(ctx, "", "teacher", "", "", 20, 0)
	if total != 1 || teachers[0].ID != bob.ID {
		t.Fatalf("teacher filter: total=%d", total)
	}

	// Remove role.
	if err := svc.SetUserRole(ctx, bob.ID, "teacher", false); err != nil {
		t.Fatal(err)
	}
	if _, total, _ := svc.ListUsers(ctx, "", "teacher", "", "", 20, 0); total != 0 {
		t.Fatalf("after remove teacher total=%d, want 0", total)
	}

	// Validation: unmanageable role + unknown user.
	if err := svc.SetUserRole(ctx, bob.ID, "moderator", true); err == nil {
		t.Fatal("expected error for unmanageable role")
	}
	if err := svc.SetUserRole(ctx, "nope", "teacher", true); err == nil {
		t.Fatal("expected error for unknown user")
	}
}

func TestBulkRoleAndUserDetail(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	a, _ := svc.Register(ctx, usecase.RegisterInput{Email: "a@x.com", Password: "pass1234", DisplayName: "A"})
	b, _ := svc.Register(ctx, usecase.RegisterInput{Email: "b@x.com", Password: "pass1234", DisplayName: "B"})

	// Bulk-assign teacher to both.
	upd, skip, err := svc.BulkSetUserRole(ctx, []string{a.ID, b.ID}, "teacher", true, "admin-actor")
	if err != nil || upd != 2 || skip != 0 {
		t.Fatalf("bulk assign: upd=%d skip=%d err=%v", upd, skip, err)
	}
	if _, total, _ := svc.ListUsers(ctx, "", "teacher", "", "", 20, 0); total != 2 {
		t.Fatalf("teacher total=%d, want 2", total)
	}

	// Actor's own admin role is never removed in bulk.
	upd, skip, _ = svc.BulkSetUserRole(ctx, []string{a.ID}, "admin", false, a.ID)
	if upd != 0 || skip != 1 {
		t.Fatalf("self-protect: upd=%d skip=%d", upd, skip)
	}

	// Detail carries roles + identity.
	d, err := svc.UserDetail(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if d.Email != "a@x.com" {
		t.Fatalf("detail email=%s", d.Email)
	}
	hasTeacher := false
	for _, r := range d.Roles {
		if r == "teacher" {
			hasTeacher = true
		}
	}
	if !hasTeacher {
		t.Fatal("detail should include teacher role")
	}
	if _, err := svc.UserDetail(ctx, "missing"); err == nil {
		t.Fatal("expected error for missing user")
	}
}

type fakeSubLookup struct{ active []string }

func (f fakeSubLookup) UserSubscriptionStatuses(_ context.Context, ids []string) (map[string][]string, error) {
	set := map[string]bool{}
	for _, a := range f.active {
		set[a] = true
	}
	out := map[string][]string{}
	for _, id := range ids {
		if set[id] {
			out[id] = []string{"classes"}
		}
	}
	return out, nil
}
func (f fakeSubLookup) ActiveSubscriberIDs(_ context.Context) ([]string, error) { return f.active, nil }

func TestListUsersSubscriptionFilter(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	a, _ := svc.Register(ctx, usecase.RegisterInput{Email: "suba@x.com", Password: "pass1234", DisplayName: "SubA"})
	_, _ = svc.Register(ctx, usecase.RegisterInput{Email: "nosub@x.com", Password: "pass1234", DisplayName: "NoSub"})
	svc.SetSubscriptionLookup(fakeSubLookup{active: []string{a.ID}})

	active, total, _ := svc.ListUsers(ctx, "", "", "active", "", 20, 0)
	if total != 1 || active[0].ID != a.ID {
		t.Fatalf("active filter: total=%d", total)
	}
	if len(active[0].Subscriptions) == 0 {
		t.Fatal("active user should carry subscriptions")
	}
	_, noneTotal, _ := svc.ListUsers(ctx, "", "", "none", "", 20, 0)
	if noneTotal != 1 {
		t.Fatalf("none filter: total=%d, want 1", noneTotal)
	}
	if _, all, _ := svc.ListUsers(ctx, "", "", "", "", 20, 0); all != 2 {
		t.Fatalf("no filter total=%d, want 2", all)
	}
}

func TestListUsersSortAndStatus(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	// Register in a name order that differs from creation order.
	charlie, _ := svc.Register(ctx, usecase.RegisterInput{Email: "c@x.com", Password: "pass1234", DisplayName: "Charlie"})
	_, _ = svc.Register(ctx, usecase.RegisterInput{Email: "a@x.com", Password: "pass1234", DisplayName: "Aaron"})
	_, _ = svc.Register(ctx, usecase.RegisterInput{Email: "b@x.com", Password: "pass1234", DisplayName: "Bella"})

	// Name sort: Aaron, Bella, Charlie.
	byName, _, _ := svc.ListUsers(ctx, "", "", "", "name", 20, 0)
	if len(byName) != 3 || byName[0].DisplayName != "Aaron" || byName[2].DisplayName != "Charlie" {
		t.Fatalf("name sort wrong: %v", []string{byName[0].DisplayName, byName[1].DisplayName, byName[2].DisplayName})
	}
	// Oldest sort: Charlie was registered first.
	oldest, _, _ := svc.ListUsers(ctx, "", "", "", "oldest", 20, 0)
	if oldest[0].ID != charlie.ID {
		t.Fatalf("oldest sort: first=%s, want charlie", oldest[0].DisplayName)
	}

	// Suspend blocks login; reactivate restores it.
	if _, err := svc.Login(ctx, "c@x.com", "pass1234", "d"); err != nil {
		t.Fatal("charlie should log in before suspension")
	}
	if err := svc.SetUserStatus(ctx, charlie.ID, "suspended"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Login(ctx, "c@x.com", "pass1234", "d"); err == nil {
		t.Fatal("suspended user must not log in")
	}
	if err := svc.SetUserStatus(ctx, charlie.ID, "active"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Login(ctx, "c@x.com", "pass1234", "d"); err != nil {
		t.Fatal("reactivated user should log in")
	}
	// Invalid status + unknown user.
	if err := svc.SetUserStatus(ctx, charlie.ID, "frozen"); err == nil {
		t.Fatal("expected invalid status error")
	}
	if err := svc.SetUserStatus(ctx, "nope", "suspended"); err == nil {
		t.Fatal("expected unknown user error")
	}
}

// A learner can study English, German, French or Turkish; anything else is
// rejected instead of silently stored.
func TestUpdateProfileAcceptsOnlySupportedTargetLanguages(t *testing.T) {
	s := newService()
	ctx := context.Background()
	u, err := s.Register(ctx, usecase.RegisterInput{Email: "lang@x.com", Password: "pw", DisplayName: "L"})
	if err != nil {
		t.Fatal(err)
	}
	for _, lang := range []string{"en", "de", "fr", "tr"} {
		got, err := s.UpdateProfile(ctx, u.ID, usecase.UpdateProfileInput{TargetLang: lang})
		if err != nil || got.TargetLang != lang {
			t.Fatalf("target %q should be accepted: %v %+v", lang, err, got)
		}
	}
	for _, lang := range []string{"ar", "es", "xx"} {
		if _, err := s.UpdateProfile(ctx, u.ID, usecase.UpdateProfileInput{TargetLang: lang}); err != domain.ErrInvalidTargetLang {
			t.Fatalf("target %q should be rejected, got %v", lang, err)
		}
	}
}
