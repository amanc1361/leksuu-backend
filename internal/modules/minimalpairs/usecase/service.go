package usecase

import (
	"context"

	"github.com/leksuu/platform/internal/modules/minimalpairs/domain"
)

type Service struct{ repo domain.Repository }

func NewService(repo domain.Repository) *Service { return &Service{repo: repo} }

// ListContrasts returns all drillable sound contrasts (with their pairs).
func (s *Service) ListContrasts(ctx context.Context) ([]domain.Contrast, error) {
	return s.repo.List(ctx)
}

// Contrast returns one contrast by id, or a not-found error.
func (s *Service) Contrast(ctx context.Context, id string) (*domain.Contrast, error) {
	return s.repo.Get(ctx, id)
}
