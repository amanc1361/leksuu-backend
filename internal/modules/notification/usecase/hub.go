package usecase

import (
	"sync"

	"github.com/leksuu/platform/internal/modules/notification/domain"
)

// Hub fans out live notifications to a user's connected SSE streams. Each browser
// tab subscribes; Publish delivers to all of that user's subscribers without
// blocking (a slow/closed subscriber is simply skipped).
type Hub struct {
	mu   sync.RWMutex
	subs map[string]map[chan *domain.Notification]struct{}
}

func NewHub() *Hub { return &Hub{subs: map[string]map[chan *domain.Notification]struct{}{}} }

// Subscribe registers a stream for a user and returns a receive channel plus an
// unsubscribe func to call on disconnect.
func (h *Hub) Subscribe(userID string) (<-chan *domain.Notification, func()) {
	ch := make(chan *domain.Notification, 8)
	h.mu.Lock()
	if h.subs[userID] == nil {
		h.subs[userID] = map[chan *domain.Notification]struct{}{}
	}
	h.subs[userID][ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		if set := h.subs[userID]; set != nil {
			delete(set, ch)
			if len(set) == 0 {
				delete(h.subs, userID)
			}
		}
		h.mu.Unlock()
		close(ch)
	}
}

// Publish delivers a notification to all of a user's live subscribers.
func (h *Hub) Publish(userID string, n *domain.Notification) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subs[userID] {
		select {
		case ch <- n:
		default: // subscriber buffer full; drop (REST list remains source of truth)
		}
	}
}
