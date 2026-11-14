package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/leksuu/platform/internal/modules/auth/domain"
	"github.com/leksuu/platform/internal/modules/auth/repo"
	"github.com/leksuu/platform/internal/modules/auth/security"
	"github.com/leksuu/platform/internal/modules/auth/usecase"
)

type failingOnboardingRepo struct{ domain.UserRepository }

func (r failingOnboardingRepo) CompleteOnboarding(context.Context, string, domain.OnboardingAnswers, bool) (domain.OnboardingState, error) {
	return domain.OnboardingState{}, errors.New("simulated database write failure")
}

func TestOnboardingWriteFailureDoesNotCompleteAccount(t *testing.T) {
	users := failingOnboardingRepo{repo.NewMemoryUserRepo()}
	svc := usecase.NewService(users, repo.NewMemoryTokenRepo(),
		security.NewPasswordHasher(), security.NewJWT([]byte("test"), 15*time.Minute, time.Now),
		security.NewRefreshFactory(), time.Now, usecase.Config{RefreshTTL: 24 * time.Hour}, nil)
	ctx := context.Background()
	u, err := svc.Register(ctx, usecase.RegisterInput{Email: "failure@sample.com", Password: "pw", DisplayName: "Failure"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.CompleteOnboarding(ctx, u.ID, domain.OnboardingAnswers{Reason: "study", Level: "A1", Goal: 5, Target: "en", UILang: "fa"}, false)
	if err == nil {
		t.Fatal("write failure was hidden")
	}
	state, err := svc.Onboarding(ctx, u.ID)
	if err != nil || state.Completed {
		t.Fatalf("failed write completed account: %+v %v", state, err)
	}
}

func TestOnboardingIsAccountScopedAndRetrySafe(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	first, err := svc.Register(ctx, usecase.RegisterInput{Email: "a@sample.com", Password: "pw", DisplayName: "A"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Register(ctx, usecase.RegisterInput{Email: "b@sample.com", Password: "pw", DisplayName: "B"})
	if err != nil {
		t.Fatal(err)
	}
	initial, err := svc.Onboarding(ctx, first.ID)
	if err != nil || initial.Completed {
		t.Fatalf("new account should not be onboarded: %+v %v", initial, err)
	}
	answers := domain.OnboardingAnswers{Reason: "study", Level: "A1", Goal: 10, Target: "en", UILang: "ckb"}
	saved, err := svc.CompleteOnboarding(ctx, first.ID, answers, false)
	if err != nil || !saved.Completed || saved.Skipped || saved.Answers.UILang != "ckb" || saved.CompletedAt == nil {
		t.Fatalf("answers not saved: %+v %v", saved, err)
	}
	// A client retry does not replace the already stored choices.
	again, err := svc.CompleteOnboarding(ctx, first.ID, domain.OnboardingAnswers{Reason: "fun", Level: "A2", Goal: 20, Target: "de", UILang: "fa"}, false)
	if err != nil || again.Answers.Reason != "study" {
		t.Fatalf("retry overwrote prior answers: %+v %v", again, err)
	}
	other, err := svc.Onboarding(ctx, second.ID)
	if err != nil || other.Completed {
		t.Fatalf("another account inherited onboarding: %+v %v", other, err)
	}
	skipped, err := svc.CompleteOnboarding(ctx, second.ID, domain.OnboardingAnswers{}, true)
	if err != nil || !skipped.Completed || !skipped.Skipped {
		t.Fatalf("skip not saved: %+v %v", skipped, err)
	}
}

func TestOnboardingRejectsIncompleteSelections(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	u, err := svc.Register(ctx, usecase.RegisterInput{Email: "c@sample.com", Password: "pw", DisplayName: "C"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CompleteOnboarding(ctx, u.ID, domain.OnboardingAnswers{Reason: "study", Level: "A1", Goal: 5, Target: "en", UILang: "xx"}, false); err == nil {
		t.Fatal("invalid UI language accepted")
	}
	state, err := svc.Onboarding(ctx, u.ID)
	if err != nil || state.Completed {
		t.Fatalf("invalid request should not persist: %+v %v", state, err)
	}
}
