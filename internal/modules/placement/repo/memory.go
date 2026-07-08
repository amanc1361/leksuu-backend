// Package repo: in-memory placement store for dev/tests.
package repo

import (
	"context"
	"sync"

	"github.com/leksuu/platform/internal/modules/placement/domain"
)

type MemoryRepo struct {
	mu      sync.RWMutex
	results map[string]domain.Result
}

func NewMemoryRepo() *MemoryRepo { return &MemoryRepo{results: map[string]domain.Result{}} }

func (r *MemoryRepo) Get(_ context.Context, userID string) (*domain.Result, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if v, ok := r.results[userID]; ok {
		cp := v
		return &cp, nil
	}
	return nil, nil
}

func (r *MemoryRepo) Upsert(_ context.Context, res *domain.Result) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.results[res.UserID] = *res
	return nil
}
