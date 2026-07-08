// Package usecase scores a recording via a swappable Scorer port. The same code
// runs against a deterministic stub (dev / before a GPU) or an HTTP scoring
// service (Whisper + GOP/wav2vec2) once one is available.
package usecase

import (
	"context"
	"strings"

	"github.com/leksuu/platform/internal/modules/pronunciation/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
)

// Scorer assesses pronunciation. Name identifies the backend.
type Scorer interface {
	Name() string
	Score(ctx context.Context, in domain.ScoreInput) (*domain.Assessment, error)
}

// Rewarder grants XP (the gamification service satisfies it structurally).
type Rewarder interface {
	Award(ctx context.Context, userID string, xp int, reason string) error
}

// GoodScore is the threshold at which a line counts as well-pronounced.
const GoodScore = 85

const goodReward = 3

type Service struct {
	scorer   Scorer
	repo     domain.Repository
	rewarder Rewarder
}

func NewService(s Scorer, repo domain.Repository, rewarder Rewarder) *Service {
	return &Service{scorer: s, repo: repo, rewarder: rewarder}
}

func (s *Service) ProviderName() string { return s.scorer.Name() }

// Score assesses a recording and (when an item is given) persists the result,
// keeping the best score per line and awarding XP the first time a line is
// pronounced well. Persistence is best-effort: a storage error never blocks
// returning the assessment to the learner.
func (s *Service) Score(ctx context.Context, userID, itemID string, line int, in domain.ScoreInput) (*domain.Assessment, *domain.LineScore, error) {
	if strings.TrimSpace(in.Expected) == "" || len(in.Audio) == 0 {
		return nil, nil, apperr.ErrBadRequest
	}
	if in.Lang == "" {
		in.Lang = "en"
	}
	a, err := s.scorer.Score(ctx, in)
	if err != nil {
		return nil, nil, err
	}
	if s.repo == nil || itemID == "" {
		return a, nil, nil
	}
	ls, prevBest, err := s.repo.Upsert(ctx, userID, itemID, line, a.Overall)
	if err != nil {
		return a, nil, nil
	}
	if s.rewarder != nil && prevBest < GoodScore && a.Overall >= GoodScore {
		_ = s.rewarder.Award(ctx, userID, goodReward, "pronunciation")
	}
	return a, &ls, nil
}

// Scores returns the user's persisted line scores for an item.
func (s *Service) Scores(ctx context.Context, userID, itemID string) ([]domain.LineScore, error) {
	if s.repo == nil {
		return nil, nil
	}
	return s.repo.ByItem(ctx, userID, itemID)
}

// Summary aggregates the user's pronunciation across all items.
func (s *Service) Summary(ctx context.Context, userID string) (domain.Summary, error) {
	if s.repo == nil {
		return domain.Summary{}, nil
	}
	return s.repo.Summary(ctx, userID, GoodScore)
}
