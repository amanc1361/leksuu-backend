package repo

import (
	"context"
	"sort"
	"sync"

	"github.com/leksuu/platform/internal/modules/recording/domain"
)

// MemoryRepo is the in-memory recording store (dev/tests).
type MemoryRepo struct {
	mu   sync.RWMutex
	byID map[string]*domain.Recording
}

func NewMemoryRepo() *MemoryRepo { return &MemoryRepo{byID: map[string]*domain.Recording{}} }

func (r *MemoryRepo) Create(_ context.Context, rec *domain.Recording) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *rec
	r.byID[rec.ID] = &cp
	return nil
}

func (r *MemoryRepo) Get(_ context.Context, id string) (*domain.Recording, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rec, ok := r.byID[id]
	if !ok {
		return nil, nil
	}
	cp := *rec
	return &cp, nil
}

func (r *MemoryRepo) Update(_ context.Context, rec *domain.Recording) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *rec
	r.byID[rec.ID] = &cp
	return nil
}

func (r *MemoryRepo) ListByClass(_ context.Context, classID string) ([]domain.Recording, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []domain.Recording
	for _, rec := range r.byID {
		if rec.ClassID == classID {
			out = append(out, *rec)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (r *MemoryRepo) ListAll(_ context.Context) ([]domain.Recording, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.Recording, 0, len(r.byID))
	for _, rec := range r.byID {
		out = append(out, *rec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (r *MemoryRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byID, id)
	return nil
}
