package usecase

import (
	"context"

	"github.com/leksuu/platform/internal/modules/auth/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
)

func (s *Service) Onboarding(ctx context.Context, userID string) (domain.OnboardingState, error) {
	return s.users.GetOnboarding(ctx, userID)
}

func (s *Service) CompleteOnboarding(ctx context.Context, userID string, a domain.OnboardingAnswers, skipped bool) (domain.OnboardingState, error) {
	if !skipped {
		if a.UILang != "fa" && a.UILang != "ckb" && a.UILang != "en" {
			return domain.OnboardingState{}, apperr.ErrBadRequest
		}
		targetAllowed := domain.IsSupportedTargetLang(a.Target)
		if s.targetLangOK != nil {
			var err error
			targetAllowed, err = s.targetLangOK(ctx, a.Target)
			if err != nil {
				return domain.OnboardingState{}, err
			}
		}
		if !targetAllowed {
			return domain.OnboardingState{}, domain.ErrInvalidTargetLang
		}
		switch a.Reason {
		case "travel", "work", "study", "migration", "fun":
		default:
			return domain.OnboardingState{}, apperr.ErrBadRequest
		}
		if a.Level != "" && a.Level != "A1" && a.Level != "A2" && a.Level != "B1" && a.Level != "B2" && a.Level != "C1" && a.Level != "C2" {
			return domain.OnboardingState{}, apperr.ErrBadRequest
		}
		if a.Goal != 5 && a.Goal != 10 && a.Goal != 15 && a.Goal != 20 {
			return domain.OnboardingState{}, apperr.ErrBadRequest
		}
		if !a.WantTest && a.Level == "" {
			return domain.OnboardingState{}, apperr.ErrBadRequest
		}
	}
	return s.users.CompleteOnboarding(ctx, userID, a, skipped)
}
