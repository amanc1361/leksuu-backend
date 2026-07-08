package repo

import (
	"context"
	"sort"
	"sync"

	"github.com/leksuu/platform/internal/modules/notification/domain"
)

type MemoryNotificationRepo struct {
	mu    sync.RWMutex
	items map[string]*domain.Notification
	prefs map[string]map[string]bool // userID -> type -> enabled (overrides only)
}

func NewMemoryNotificationRepo() *MemoryNotificationRepo {
	return &MemoryNotificationRepo{items: map[string]*domain.Notification{}, prefs: map[string]map[string]bool{}}
}

func (r *MemoryNotificationRepo) GetPrefs(_ context.Context, userID string) (map[string]bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := map[string]bool{}
	for k, v := range r.prefs[userID] {
		out[k] = v
	}
	return out, nil
}

func (r *MemoryNotificationRepo) SetPref(_ context.Context, userID, ntype string, enabled bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.prefs[userID] == nil {
		r.prefs[userID] = map[string]bool{}
	}
	r.prefs[userID][ntype] = enabled
	return nil
}

func (r *MemoryNotificationRepo) Create(_ context.Context, n *domain.Notification) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *n
	r.items[n.ID] = &cp
	return nil
}

func (r *MemoryNotificationRepo) List(_ context.Context, userID string, limit, offset int) ([]domain.Notification, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []domain.Notification{}
	for _, n := range r.items {
		if n.UserID == userID {
			out = append(out, *n)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if offset > len(out) {
		offset = len(out)
	}
	out = out[offset:]
	if limit > 0 && limit < len(out) {
		out = out[:limit]
	}
	return out, nil
}

func (r *MemoryNotificationRepo) UnreadCount(_ context.Context, userID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	n := 0
	for _, it := range r.items {
		if it.UserID == userID && !it.Read {
			n++
		}
	}
	return n, nil
}

func (r *MemoryNotificationRepo) MarkRead(_ context.Context, userID, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if it, ok := r.items[id]; ok && it.UserID == userID {
		it.Read = true
	}
	return nil
}

func (r *MemoryNotificationRepo) MarkAllRead(_ context.Context, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, it := range r.items {
		if it.UserID == userID {
			it.Read = true
		}
	}
	return nil
}
