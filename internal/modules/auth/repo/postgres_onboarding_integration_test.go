//go:build pgx

package repo

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/leksuu/platform/internal/modules/auth/domain"
)

func TestPostgresOnboardingPersistsAndIsolatesAccounts(t *testing.T) {
	if os.Getenv("LEKSUU_PHASE1A_ISOLATED") != "1" {
		t.Skip("explicit Phase 1A isolated database opt-in required")
	}
	dsn := os.Getenv("LEKSUU_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("LEKSUU_TEST_DATABASE_URL is unset")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConnConfig.Host != "127.0.0.1" || cfg.ConnConfig.Port != 55434 || cfg.ConnConfig.Database != "phase1a_test" || cfg.ConnConfig.User != "phase1a" {
		t.Fatalf("refusing non-isolated database target: host=%s port=%d database=%s user=%s",
			cfg.ConnConfig.Host, cfg.ConnConfig.Port, cfg.ConnConfig.Database, cfg.ConnConfig.User)
	}
	ctx := context.Background()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var db string
	if err := pool.QueryRow(ctx, `SELECT current_database()`).Scan(&db); err != nil || db != "phase1a_test" {
		t.Fatalf("database identity: %s %v", db, err)
	}
	repo := NewPostgresUserRepo(pool)
	makeUser := func(n int) string {
		t.Helper()
		var id string
		email := fmt.Sprintf("phase1a-%d-%d@example.invalid", time.Now().UnixNano(), n)
		if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,display_name,native_lang,target_lang,cefr_level)
			VALUES($1,'fixture-hash','Fixture','fa','en','A1') RETURNING id`, email).Scan(&id); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, id) })
		return id
	}
	first := makeUser(1)
	second := makeUser(2)
	firstProfile, err := repo.ByID(ctx, first)
	if err != nil || firstProfile.TranslationLang != "fa" {
		t.Fatalf("new account translation default: %+v %v", firstProfile, err)
	}
	if err := repo.UpdateProfile(ctx, first, domain.ProfileFields{DisplayName: firstProfile.DisplayName, NativeLang: firstProfile.NativeLang, TargetLang: firstProfile.TargetLang, CEFR: firstProfile.CEFR, TranslationLang: "ckb"}); err != nil {
		t.Fatal(err)
	}
	reloadedProfile, err := NewPostgresUserRepo(pool).ByID(ctx, first)
	if err != nil || reloadedProfile.TranslationLang != "ckb" {
		t.Fatalf("translation not durable: %+v %v", reloadedProfile, err)
	}
	secondProfile, err := repo.ByID(ctx, second)
	if err != nil || secondProfile.TranslationLang != "fa" {
		t.Fatalf("translation leaked between accounts: %+v %v", secondProfile, err)
	}
	before, err := repo.GetOnboarding(ctx, first)
	if err != nil || before.Completed {
		t.Fatalf("new user state: %+v %v", before, err)
	}
	answers := domain.OnboardingAnswers{UILang: "ckb", Target: "en", Reason: "study", Level: "A1", Goal: 10}
	saved, err := repo.CompleteOnboarding(ctx, first, answers, false)
	if err != nil || !saved.Completed || saved.CompletedAt == nil || saved.Answers.UILang != "ckb" {
		t.Fatalf("first completion: %+v %v", saved, err)
	}
	// A new repository instance models the same database after an API restart.
	reloaded, err := NewPostgresUserRepo(pool).GetOnboarding(ctx, first)
	if err != nil || !reloaded.Completed || reloaded.Answers.Reason != "study" {
		t.Fatalf("reloaded state: %+v %v", reloaded, err)
	}
	retry, err := repo.CompleteOnboarding(ctx, first, domain.OnboardingAnswers{Reason: "fun"}, false)
	if err != nil || retry.Answers.Reason != "study" || !retry.CompletedAt.Equal(*saved.CompletedAt) {
		t.Fatalf("retry changed first write: %+v %v", retry, err)
	}
	other, err := repo.GetOnboarding(ctx, second)
	if err != nil || other.Completed {
		t.Fatalf("second user inherited completion: %+v %v", other, err)
	}
	if _, err := repo.CompleteOnboarding(ctx, "00000000-0000-4000-8000-000000000999", answers, false); err == nil {
		t.Fatal("foreign-key failure was hidden")
	}
	other, err = repo.GetOnboarding(ctx, second)
	if err != nil || other.Completed {
		t.Fatalf("failed write changed another user: %+v %v", other, err)
	}
}
