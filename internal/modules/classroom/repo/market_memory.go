package repo

import (
	"context"
	"time"

	"github.com/leksuu/platform/internal/modules/classroom/domain"
)

type classMarket struct {
	prices    map[string]int
	seats     map[string]domain.EnrollmentPrice // classID|studentID
	checkouts map[string]domain.ClassCheckout
}

func (r *MemoryClassRepo) mk() *classMarket {
	if r.market == nil {
		r.market = &classMarket{prices: map[string]int{}, seats: map[string]domain.EnrollmentPrice{}, checkouts: map[string]domain.ClassCheckout{}}
	}
	return r.market
}

func seatKey(classID, studentID string) string { return classID + "|" + studentID }

func (r *MemoryClassRepo) ClassPrice(_ context.Context, classID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.market == nil {
		return 0, nil
	}
	return r.market.prices[classID], nil
}

func (r *MemoryClassRepo) SetClassPrice(_ context.Context, classID string, price int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mk().prices[classID] = price
	return nil
}

func (r *MemoryClassRepo) SaveEnrollmentPrice(_ context.Context, p domain.EnrollmentPrice) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mk().seats[seatKey(p.ClassID, p.StudentID)] = p
	return nil
}

func (r *MemoryClassRepo) EnrollmentPrice(_ context.Context, classID, studentID string) (*domain.EnrollmentPrice, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.market == nil {
		return nil, nil
	}
	p, ok := r.market.seats[seatKey(classID, studentID)]
	if !ok {
		return nil, nil
	}
	return &p, nil
}

func (r *MemoryClassRepo) EnrollmentPrices(_ context.Context, classIDs []string) (map[string][]domain.EnrollmentPrice, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := map[string][]domain.EnrollmentPrice{}
	if r.market == nil {
		return out, nil
	}
	want := map[string]bool{}
	for _, id := range classIDs {
		want[id] = true
	}
	for _, p := range r.market.seats {
		if want[p.ClassID] {
			out[p.ClassID] = append(out[p.ClassID], p)
		}
	}
	return out, nil
}

func (r *MemoryClassRepo) SetEnrollmentRefunded(_ context.Context, classID, studentID string, refunded int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	m := r.mk()
	if p, ok := m.seats[seatKey(classID, studentID)]; ok {
		p.RefundedToman = refunded
		m.seats[seatKey(classID, studentID)] = p
	}
	return nil
}

func (r *MemoryClassRepo) CreateCheckout(_ context.Context, c domain.ClassCheckout) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mk().checkouts[c.ID] = c
	return nil
}

func (r *MemoryClassRepo) Checkout(_ context.Context, id string) (*domain.ClassCheckout, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.market == nil {
		return nil, nil
	}
	c, ok := r.market.checkouts[id]
	if !ok {
		return nil, nil
	}
	return &c, nil
}

func (r *MemoryClassRepo) HeldSeats(_ context.Context, classID string, since time.Time) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	n := 0
	if r.market != nil {
		for _, c := range r.market.checkouts {
			if c.ClassID == classID && c.Status == domain.CheckoutPending && c.CreatedAt.After(since) {
				n++
			}
		}
	}
	return n, nil
}

func (r *MemoryClassRepo) SetCheckout(_ context.Context, id, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	m := r.mk()
	if c, ok := m.checkouts[id]; ok {
		c.Status = status
		m.checkouts[id] = c
	}
	return nil
}

var _ domain.MarketStore = (*MemoryClassRepo)(nil)
