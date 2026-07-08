package repo

import (
	"context"
	"sync"

	"github.com/leksuu/platform/internal/modules/shadowing/domain"
)

type MemoryShadowRepo struct {
	mu sync.Mutex
	m  map[string]map[string]int // userID|itemID -> {line: reps}
}

func NewMemoryShadowRepo() *MemoryShadowRepo {
	return &MemoryShadowRepo{m: map[string]map[string]int{}}
}

func key(userID, itemID string) string { return userID + "|" + itemID }

func clone(src map[string]int) map[string]int {
	out := make(map[string]int, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func (r *MemoryShadowRepo) Get(_ context.Context, userID, itemID string) (*domain.Progress, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return &domain.Progress{ItemID: itemID, Reps: clone(r.m[key(userID, itemID)])}, nil
}

func (r *MemoryShadowRepo) AddRep(_ context.Context, userID, itemID, line string) (*domain.Progress, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := key(userID, itemID)
	if r.m[k] == nil {
		r.m[k] = map[string]int{}
	}
	r.m[k][line]++
	return &domain.Progress{ItemID: itemID, Reps: clone(r.m[k])}, nil
}
