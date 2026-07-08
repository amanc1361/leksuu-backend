package repo

import (
	"context"
	"sort"
	"sync"

	"github.com/leksuu/platform/internal/modules/dictionary/domain"
	"github.com/leksuu/platform/internal/shared/id"
)

type MemoryDictRepo struct {
	mu sync.Mutex
	m  map[string]*domain.Provider
}

func NewMemoryDictRepo() *MemoryDictRepo { return &MemoryDictRepo{m: map[string]*domain.Provider{}} }

func (r *MemoryDictRepo) all() []domain.Provider {
	out := make([]domain.Provider, 0, len(r.m))
	for _, p := range r.m {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Position != out[j].Position {
			return out[i].Position < out[j].Position
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func (r *MemoryDictRepo) List(_ context.Context) ([]domain.Provider, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.all(), nil
}

func (r *MemoryDictRepo) Enabled(_ context.Context) ([]domain.Provider, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domain.Provider
	for _, p := range r.all() {
		if p.Enabled {
			out = append(out, p)
		}
	}
	return out, nil
}

func (r *MemoryDictRepo) Get(_ context.Context, pid string) (*domain.Provider, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.m[pid]; ok {
		cp := *p
		return &cp, nil
	}
	return nil, domain.ErrProviderNotFound
}

func (r *MemoryDictRepo) Create(_ context.Context, p *domain.Provider) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p.ID = id.New()
	cp := *p
	r.m[p.ID] = &cp
	return nil
}

func (r *MemoryDictRepo) Update(_ context.Context, p *domain.Provider) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.m[p.ID]; !ok {
		return domain.ErrProviderNotFound
	}
	cp := *p
	r.m[p.ID] = &cp
	return nil
}

func (r *MemoryDictRepo) Delete(_ context.Context, pid string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.m[pid]; !ok {
		return domain.ErrProviderNotFound
	}
	delete(r.m, pid)
	return nil
}
