package repo

import (
	"context"
	"sort"
	"sync"

	"github.com/leksuu/platform/internal/modules/settings/domain"
)

// MemorySettingsRepo keeps the single settings record in memory.
type MemorySettingsRepo struct {
	mu sync.RWMutex
	s  domain.Settings
}

func NewMemorySettingsRepo() *MemorySettingsRepo {
	return &MemorySettingsRepo{s: domain.Defaults()}
}

func (r *MemorySettingsRepo) Get(_ context.Context) (*domain.Settings, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cp := r.s
	return &cp, nil
}

func (r *MemorySettingsRepo) Save(_ context.Context, s *domain.Settings) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.s = *s
	return nil
}

// MemoryPlanRepo holds subscription plans in memory (seeded with the defaults
// that mirror migration 0037).
type MemoryPlanRepo struct {
	mu    sync.RWMutex
	plans []domain.Plan
}

func NewMemoryPlanRepo() *MemoryPlanRepo {
	return &MemoryPlanRepo{plans: []domain.Plan{
		{Product: "courses", ID: "monthly", Title: "ماهانه", AmountToman: 99000, Days: 30, Position: 1, Active: true},
		{Product: "courses", ID: "yearly", Title: "سالانه", AmountToman: 890000, Days: 365, Position: 2, Active: true},
		{Product: "classes", ID: "monthly", Title: "ماهانه", AmountToman: 149000, Days: 30, Position: 1, Active: true},
		{Product: "classes", ID: "yearly", Title: "سالانه", AmountToman: 1490000, Days: 365, Position: 2, Active: true},
	}}
}

func (r *MemoryPlanRepo) List(_ context.Context) ([]domain.Plan, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := append([]domain.Plan(nil), r.plans...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Product != out[j].Product {
			return out[i].Product < out[j].Product
		}
		return out[i].Position < out[j].Position
	})
	return out, nil
}

func (r *MemoryPlanRepo) ListActiveByProduct(_ context.Context, product string) ([]domain.Plan, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []domain.Plan
	for _, p := range r.plans {
		if p.Product == product && p.Active {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Position < out[j].Position })
	return out, nil
}

func (r *MemoryPlanRepo) Upsert(_ context.Context, p domain.Plan) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.plans {
		if r.plans[i].Product == p.Product && r.plans[i].ID == p.ID {
			r.plans[i] = p
			return nil
		}
	}
	r.plans = append(r.plans, p)
	return nil
}

func (r *MemoryPlanRepo) Delete(_ context.Context, product, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.plans[:0]
	for _, p := range r.plans {
		if p.Product == product && p.ID == id {
			continue
		}
		out = append(out, p)
	}
	r.plans = out
	return nil
}
