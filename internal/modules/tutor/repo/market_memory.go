package repo

import (
	"context"
	"sort"

	"github.com/leksuu/platform/internal/modules/tutor/domain"
)

// In-memory MarketStore (tests and the dev server).
type marketMem struct {
	prices    map[string]int
	bookings  map[string]domain.BookingPrice
	credits   map[string]int
	ledger    []CreditLine
	checkouts map[string]domain.Checkout
}

// CreditLine is one ledger entry (exposed for tests).
type CreditLine struct {
	UserID    string
	Delta     int
	Reason    string
	BookingID string
}

func (r *MemoryTutorRepo) market() *marketMem {
	if r.mkt == nil {
		r.mkt = &marketMem{prices: map[string]int{}, bookings: map[string]domain.BookingPrice{}, credits: map[string]int{}, checkouts: map[string]domain.Checkout{}}
	}
	return r.mkt
}

func (r *MemoryTutorRepo) TutorPrice(_ context.Context, tutorID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.mkt == nil {
		return 0, nil
	}
	return r.mkt.prices[tutorID], nil
}

func (r *MemoryTutorRepo) SetTutorPrice(_ context.Context, tutorID string, priceToman int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.market().prices[tutorID] = priceToman
	return nil
}

func (r *MemoryTutorRepo) ClaimSlot(_ context.Context, slotID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.slots[slotID]
	if !ok || s.Booked {
		return false, nil
	}
	s.Booked = true
	return true, nil
}

func (r *MemoryTutorRepo) SaveBookingPrice(_ context.Context, p domain.BookingPrice) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.market().bookings[p.BookingID] = p
	return nil
}

func (r *MemoryTutorRepo) BookingPrices(_ context.Context, ids []string) (map[string]domain.BookingPrice, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := map[string]domain.BookingPrice{}
	if r.mkt == nil {
		return out, nil
	}
	for _, id := range ids {
		if p, ok := r.mkt.bookings[id]; ok {
			out[id] = p
		}
	}
	return out, nil
}

func (r *MemoryTutorRepo) SetRefunded(_ context.Context, bookingID string, refunded int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	m := r.market()
	if p, ok := m.bookings[bookingID]; ok {
		p.RefundedToman = refunded
		m.bookings[bookingID] = p
	}
	return nil
}

func (r *MemoryTutorRepo) CreditBalance(_ context.Context, userID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.mkt == nil {
		return 0, nil
	}
	return r.mkt.credits[userID], nil
}

func (r *MemoryTutorRepo) AdjustCredit(_ context.Context, userID string, delta int, reason, bookingID string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m := r.market()
	if m.credits[userID]+delta < 0 {
		return 0, domain.ErrInsufficientCredit
	}
	m.credits[userID] += delta
	m.ledger = append(m.ledger, CreditLine{UserID: userID, Delta: delta, Reason: reason, BookingID: bookingID})
	return m.credits[userID], nil
}

// Ledger returns a user's credit history, oldest first (tests).
func (r *MemoryTutorRepo) Ledger(userID string) []CreditLine {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []CreditLine
	if r.mkt != nil {
		for _, l := range r.mkt.ledger {
			if l.UserID == userID {
				out = append(out, l)
			}
		}
	}
	return out
}

func (r *MemoryTutorRepo) CreateCheckout(_ context.Context, c domain.Checkout) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.market().checkouts[c.ID] = c
	return nil
}

func (r *MemoryTutorRepo) Checkout(_ context.Context, id string) (*domain.Checkout, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.mkt == nil {
		return nil, nil
	}
	c, ok := r.mkt.checkouts[id]
	if !ok {
		return nil, nil
	}
	return &c, nil
}

func (r *MemoryTutorRepo) PendingCheckoutForSlot(_ context.Context, slotID string) (*domain.Checkout, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.mkt == nil {
		return nil, nil
	}
	var found []domain.Checkout
	for _, c := range r.mkt.checkouts {
		if c.SlotID == slotID && c.Status == domain.CheckoutPending {
			found = append(found, c)
		}
	}
	if len(found) == 0 {
		return nil, nil
	}
	sort.Slice(found, func(i, j int) bool { return found[i].CreatedAt.After(found[j].CreatedAt) })
	return &found[0], nil
}

func (r *MemoryTutorRepo) SetCheckout(_ context.Context, id, status, bookingID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	m := r.market()
	if c, ok := m.checkouts[id]; ok {
		c.Status, c.BookingID = status, bookingID
		m.checkouts[id] = c
	}
	return nil
}

var _ domain.MarketStore = (*MemoryTutorRepo)(nil)
