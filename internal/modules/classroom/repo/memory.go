package repo

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/leksuu/platform/internal/modules/classroom/domain"
)

type MemoryClassRepo struct {
	mu      sync.RWMutex
	classes map[string]*domain.ClassSession
	enroll  map[string]map[string]bool // classID -> set of studentID
}

func NewMemoryClassRepo() *MemoryClassRepo {
	return &MemoryClassRepo{classes: map[string]*domain.ClassSession{}, enroll: map[string]map[string]bool{}}
}

func (r *MemoryClassRepo) CreateClass(_ context.Context, c *domain.ClassSession) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *c
	r.classes[c.ID] = &cp
	return nil
}

func (r *MemoryClassRepo) GetClass(_ context.Context, id string) (*domain.ClassSession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if c, ok := r.classes[id]; ok {
		cp := *c
		return &cp, nil
	}
	return nil, nil
}

func (r *MemoryClassRepo) ListUpcoming(_ context.Context, now time.Time) ([]domain.ClassSession, error) {
	return r.filter(func(c *domain.ClassSession) bool { return c.EndUTC().After(now) }), nil
}

func (r *MemoryClassRepo) ByTutor(_ context.Context, tutorID string) ([]domain.ClassSession, error) {
	return r.filter(func(c *domain.ClassSession) bool { return c.TutorID == tutorID }), nil
}

func (r *MemoryClassRepo) ClassesForStudent(_ context.Context, studentID string) ([]domain.ClassSession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []domain.ClassSession{}
	for id, set := range r.enroll {
		if set[studentID] {
			if c, ok := r.classes[id]; ok {
				out = append(out, *c)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartUTC.Before(out[j].StartUTC) })
	return out, nil
}

func (r *MemoryClassRepo) filter(keep func(*domain.ClassSession) bool) []domain.ClassSession {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []domain.ClassSession{}
	for _, c := range r.classes {
		if keep(c) {
			out = append(out, *c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartUTC.Before(out[j].StartUTC) })
	return out
}

func (r *MemoryClassRepo) Enroll(_ context.Context, classID, studentID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.enroll[classID] == nil {
		r.enroll[classID] = map[string]bool{}
	}
	r.enroll[classID][studentID] = true
	return nil
}

func (r *MemoryClassRepo) Unenroll(_ context.Context, classID, studentID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if set := r.enroll[classID]; set != nil {
		delete(set, studentID)
	}
	return nil
}

func (r *MemoryClassRepo) IsEnrolled(_ context.Context, classID, studentID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.enroll[classID][studentID], nil
}

func (r *MemoryClassRepo) EnrollmentCount(_ context.Context, classID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.enroll[classID]), nil
}

func (r *MemoryClassRepo) Roster(_ context.Context, classID string) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []string{}
	for id := range r.enroll[classID] {
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}

func (r *MemoryClassRepo) ClassesStarting(_ context.Context, from, to time.Time) ([]domain.ClassSession, error) {
	return r.filter(func(c *domain.ClassSession) bool {
		return !c.StartUTC.Before(from) && !c.StartUTC.After(to)
	}), nil
}

func (r *MemoryClassRepo) AllClasses(_ context.Context) ([]domain.ClassSession, error) {
	return r.filter(func(*domain.ClassSession) bool { return true }), nil
}
