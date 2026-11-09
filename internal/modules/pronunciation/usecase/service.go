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

var ErrScoringUnavailable = apperr.New("pronunciation.unavailable", apperr.KindConflict)

type Service struct {
	scorer   Scorer
	repo     domain.Repository
	rewarder Rewarder
}

func NewService(s Scorer, repo domain.Repository, rewarder Rewarder) *Service {
	return &Service{scorer: s, repo: repo, rewarder: rewarder}
}

func (s *Service) ProviderName() string {
	if s.scorer == nil {
		return ""
	}
	return s.scorer.Name()
}
func (s *Service) Available() bool {
	return s.scorer != nil && s.scorer.Name() != "stub" && s.scorer.Name() != ""
}

// Score assesses a recording and (when an item is given) persists the result,
// keeping the best score per line and awarding XP the first time a line is
// pronounced well. Persistence is best-effort: a storage error never blocks
// returning the assessment to the learner.
func (s *Service) Score(ctx context.Context, userID, itemID string, line int, in domain.ScoreInput) (*domain.Assessment, *domain.LineScore, error) {
	if strings.TrimSpace(in.Expected) == "" || len(in.Audio) == 0 {
		return nil, nil, apperr.ErrBadRequest
	}
	// A text-derived demo value is never a measurement of the user's voice.
	// Preserve existing history, but do not add fake scores or grant rewards.
	if !s.Available() {
		return nil, nil, ErrScoringUnavailable
	}
	if in.Lang == "" {
		in.Lang = "en"
	}
	a, err := s.scorer.Score(ctx, in)
	if err != nil {
		return nil, nil, err
	}
	if a == nil || a.Overall < 0 || a.Overall > 100 || len(a.Words) > 1000 || len(a.Feedback) > 24 {
		return nil, nil, apperr.New("pronunciation.bad_output", apperr.KindValidation)
	}
	for _, word := range a.Words {
		if word.Score < 0 || word.Score > 100 {
			return nil, nil, apperr.New("pronunciation.bad_output", apperr.KindValidation)
		}
	}
	for _, value := range []*int{a.Accuracy, a.Fluency, a.Prosody, a.Rhythm, a.Pauses} {
		if value != nil && (*value < 0 || *value > 100) {
			return nil, nil, apperr.New("pronunciation.bad_output", apperr.KindValidation)
		}
	}
	for _, feedback := range a.Feedback {
		if len(feedback.Message) > 2000 || len(feedback.Word) > 200 || len(feedback.Dimension) > 80 {
			return nil, nil, apperr.New("pronunciation.bad_output", apperr.KindValidation)
		}
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
