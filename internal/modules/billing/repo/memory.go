package repo

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/leksuu/platform/internal/modules/billing/domain"
)

// MemoryBillingRepo keeps subscriptions, payments, and daily usage in memory.
type MemoryBillingRepo struct {
	mu      sync.Mutex
	subs    map[string]domain.Subscription // userID -> sub
	pays    map[string]domain.Payment      // authority -> payment
	usage   map[string]int                 // userID|day|action -> count
	unlocks map[string][2]string           // userID|lessonID -> {source, day}
}

func NewMemoryBillingRepo() *MemoryBillingRepo {
	return &MemoryBillingRepo{
		subs:    map[string]domain.Subscription{},
		pays:    map[string]domain.Payment{},
		usage:   map[string]int{},
		unlocks: map[string][2]string{},
	}
}

func uKey(userID, day, action string) string { return userID + "|" + day + "|" + action }

func subKey(userID, product string) string {
	if product == "" {
		product = domain.ProductCourses
	}
	return userID + "|" + product
}

func (r *MemoryBillingRepo) GetSubscription(_ context.Context, userID, product string) (*domain.Subscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.subs[subKey(userID, product)]
	if !ok {
		return nil, nil
	}
	cp := s
	return &cp, nil
}

func (r *MemoryBillingRepo) UpsertSubscription(_ context.Context, s domain.Subscription) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s.Product == "" {
		s.Product = domain.ProductCourses
	}
	r.subs[subKey(s.UserID, s.Product)] = s
	return nil
}

func (r *MemoryBillingRepo) CreatePayment(_ context.Context, p domain.Payment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pays[p.Authority] = p
	return nil
}

func (r *MemoryBillingRepo) GetPayment(_ context.Context, authority string) (*domain.Payment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.pays[authority]
	if !ok {
		return nil, nil
	}
	cp := p
	return &cp, nil
}

func (r *MemoryBillingRepo) MarkPayment(_ context.Context, authority, status, refID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.pays[authority]; ok {
		p.Status = status
		p.RefID = refID
		r.pays[authority] = p
	}
	return nil
}

func (r *MemoryBillingRepo) UsageToday(_ context.Context, userID, action, day string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.usage[uKey(userID, day, action)], nil
}

func (r *MemoryBillingRepo) IncrUsage(_ context.Context, userID, action, day string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := uKey(userID, day, action)
	r.usage[k]++
	return r.usage[k], nil
}

func unKey(userID, lessonID string) string { return userID + "|" + lessonID }

func (r *MemoryBillingRepo) LessonUnlocked(_ context.Context, userID, lessonID, today string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.unlocks[unKey(userID, lessonID)]
	if !ok {
		return false, nil
	}
	source, day := u[0], u[1]
	return source == "gem" || (source == "free" && day == today), nil
}

func (r *MemoryBillingRepo) FreeUnlocksToday(_ context.Context, userID, today string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	prefix := userID + "|"
	for k, u := range r.unlocks {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix && u[0] == "free" && u[1] == today {
			n++
		}
	}
	return n, nil
}

func (r *MemoryBillingRepo) AddUnlock(_ context.Context, userID, lessonID, source, day string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.unlocks[unKey(userID, lessonID)] = [2]string{source, day}
	return nil
}

func (r *MemoryBillingRepo) PaidPaymentTotals(_ context.Context) (map[string]int, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	by := map[string]int{}
	count := 0
	for _, p := range r.pays {
		if p.Status != "paid" {
			continue
		}
		product := p.Product
		if product == "" {
			product = domain.ProductCourses
		}
		by[product] += p.AmountToman
		count++
	}
	return by, count, nil
}

func (r *MemoryBillingRepo) ActiveSubscriptionCounts(_ context.Context, now time.Time) (map[string]int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]int{}
	for _, s := range r.subs {
		if s.Active(now) {
			product := s.Product
			if product == "" {
				product = domain.ProductCourses
			}
			out[product]++
		}
	}
	return out, nil
}

func (r *MemoryBillingRepo) ActiveProductsForUsers(_ context.Context, userIDs []string, now time.Time) (map[string][]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	want := map[string]bool{}
	for _, id := range userIDs {
		want[id] = true
	}
	out := map[string][]string{}
	for _, s := range r.subs {
		if want[s.UserID] && s.Active(now) {
			product := s.Product
			if product == "" {
				product = domain.ProductCourses
			}
			out[s.UserID] = append(out[s.UserID], product)
		}
	}
	return out, nil
}

func (r *MemoryBillingRepo) PaymentsForUser(_ context.Context, userID string) ([]domain.Payment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []domain.Payment{}
	for _, p := range r.pays {
		if p.UserID == userID {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (r *MemoryBillingRepo) SubscriptionsForUser(_ context.Context, userID string) ([]domain.Subscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []domain.Subscription{}
	for _, s := range r.subs {
		if s.UserID == userID {
			out = append(out, s)
		}
	}
	return out, nil
}

func (r *MemoryBillingRepo) ActiveSubscriberIDs(_ context.Context, now time.Time) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	seen := map[string]bool{}
	out := []string{}
	for _, s := range r.subs {
		if s.Active(now) && !seen[s.UserID] {
			seen[s.UserID] = true
			out = append(out, s.UserID)
		}
	}
	return out, nil
}

// TransitionPayment updates the status only if it is still `from`.
func (r *MemoryBillingRepo) TransitionPayment(_ context.Context, authority, from, to, refID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.pays[authority]
	if !ok || p.Status != from {
		return false, nil
	}
	p.Status, p.RefID = to, refID
	r.pays[authority] = p
	return true, nil
}
