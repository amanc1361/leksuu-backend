// Package usecase holds shadowing rules: count repetitions and award a little XP
// up to the activation threshold (so practice feeds the daily goal without
// being farmable beyond what activates a line).
package usecase

import (
	"context"

	"github.com/leksuu/platform/internal/modules/shadowing/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
)

// Rewarder grants XP (the gamification service satisfies it structurally).
type Rewarder interface {
	Award(ctx context.Context, userID string, xp int, reason string) error
}

type Service struct {
	repo     domain.Repository
	rewarder Rewarder
}

func NewService(repo domain.Repository, rewarder Rewarder) *Service {
	return &Service{repo: repo, rewarder: rewarder}
}

func (s *Service) Progress(ctx context.Context, userID, itemID string) (*domain.Progress, error) {
	if itemID == "" {
		return nil, apperr.ErrBadRequest
	}
	return s.repo.Get(ctx, userID, itemID)
}

// AddRep records one repetition of a line and returns the updated progress.
func (s *Service) AddRep(ctx context.Context, userID, itemID, line string) (*domain.Progress, error) {
	if itemID == "" || line == "" {
		return nil, apperr.ErrBadRequest
	}
	p, err := s.repo.AddRep(ctx, userID, itemID, line)
	if err != nil {
		return nil, err
	}
	// Reward progress toward activation only (1 XP per rep, capped per line).
	if s.rewarder != nil && p.Reps[line] <= domain.ActivationReps {
		_ = s.rewarder.Award(ctx, userID, 1, "shadowing")
	}
	return p, nil
}
