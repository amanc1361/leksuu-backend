package repo

import (
	"context"
	"sort"
	"sync"

	"github.com/leksuu/platform/internal/modules/assignment/domain"
)

type MemoryAssignmentRepo struct {
	mu    sync.RWMutex
	items map[string]*domain.Assignment
}

func NewMemoryAssignmentRepo() *MemoryAssignmentRepo {
	return &MemoryAssignmentRepo{items: map[string]*domain.Assignment{}}
}

func (r *MemoryAssignmentRepo) Create(_ context.Context, a *domain.Assignment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *a
	r.items[a.ID] = &cp
	return nil
}

func (r *MemoryAssignmentRepo) Get(_ context.Context, id string) (*domain.Assignment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if a, ok := r.items[id]; ok {
		cp := *a
		return &cp, nil
	}
	return nil, nil
}

func (r *MemoryAssignmentRepo) ByTutor(_ context.Context, tutorID string) ([]domain.Assignment, error) {
	return r.filter(func(a *domain.Assignment) bool { return a.TutorID == tutorID }), nil
}

func (r *MemoryAssignmentRepo) ByStudent(_ context.Context, studentID string) ([]domain.Assignment, error) {
	return r.filter(func(a *domain.Assignment) bool { return a.StudentID == studentID }), nil
}

func (r *MemoryAssignmentRepo) filter(keep func(*domain.Assignment) bool) []domain.Assignment {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []domain.Assignment{}
	for _, a := range r.items {
		if keep(a) {
			out = append(out, *a)
		}
	}
	// Newest first.
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (r *MemoryAssignmentRepo) Update(_ context.Context, a *domain.Assignment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *a
	r.items[a.ID] = &cp
	return nil
}
