package repo

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/leksuu/platform/internal/modules/tutor/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
)

// MemoryTutorRepo is an in-memory tutor store (dev/tests) seeded with demo tutors
// and a rolling window of future slots so the marketplace is never empty.
type MemoryTutorRepo struct {
	mu       sync.RWMutex
	tutors   map[string]*domain.Tutor
	order    []string
	slots    map[string]*domain.Slot
	bookings map[string]*domain.Booking
	reviews  map[string]*domain.Review // by booking id
	clock    func() time.Time
}

func NewMemoryTutorRepo() *MemoryTutorRepo {
	r := &MemoryTutorRepo{
		tutors:   map[string]*domain.Tutor{},
		slots:    map[string]*domain.Slot{},
		bookings: map[string]*domain.Booking{},
		reviews:  map[string]*domain.Review{},
		clock:    time.Now,
	}
	r.seed()
	return r
}

func (r *MemoryTutorRepo) seed() {
	add := func(t *domain.Tutor) {
		if t.Status == "" {
			t.Status = domain.StatusApproved
		}
		r.tutors[t.ID] = t
		r.order = append(r.order, t.ID)
	}
	add(&domain.Tutor{ID: "tutor-sara", Name: "Sara Ahmadi", Headline: "Friendly conversation coach",
		Bio:       "Native-level English teacher focused on speaking confidence for beginners.",
		Languages: []string{"en", "fa"}, HourlyGems: 300, AvatarURL: "", RatingAvg: 4.8, RatingCount: 24})
	add(&domain.Tutor{ID: "tutor-dilan", Name: "Dilan Karim", Headline: "Grammar & pronunciation",
		Bio:       "Patient tutor for Kurdish and Persian speakers learning English from scratch.",
		Languages: []string{"en", "ckb", "fa"}, HourlyGems: 250, AvatarURL: "", RatingAvg: 4.6, RatingCount: 11})
	add(&domain.Tutor{ID: "tutor-john", Name: "John Miller", Headline: "Exam & business English",
		Bio:       "Helps learners prepare for interviews and everyday work conversations.",
		Languages: []string{"en"}, HourlyGems: 400, AvatarURL: "", RatingAvg: 4.9, RatingCount: 37})

	// Rolling future slots: next 7 days, two slots/day per tutor.
	base := r.clock().UTC().Truncate(time.Hour)
	hours := map[string][]int{"tutor-sara": {16, 18}, "tutor-dilan": {15, 19}, "tutor-john": {17, 20}}
	n := 0
	for _, tid := range r.order {
		for d := 1; d <= 7; d++ {
			for _, h := range hours[tid] {
				day := base.AddDate(0, 0, d)
				start := time.Date(day.Year(), day.Month(), day.Day(), h, 0, 0, 0, time.UTC)
				n++
				sid := "slot-" + tid + "-" + start.Format("0102-15")
				r.slots[sid] = &domain.Slot{ID: sid, TutorID: tid, StartUTC: start, DurationMin: 30}
			}
		}
	}
}

func (r *MemoryTutorRepo) ListTutors(_ context.Context, lang string) ([]domain.Tutor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.Tutor, 0, len(r.order))
	for _, id := range r.order {
		t := r.tutors[id]
		if lang != "" {
			ok := false
			for _, l := range t.Languages {
				if l == lang {
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
		}
		out = append(out, *t)
	}
	return out, nil
}

func (r *MemoryTutorRepo) Tutor(_ context.Context, id string) (*domain.Tutor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if t, ok := r.tutors[id]; ok {
		cp := *t
		return &cp, nil
	}
	return nil, nil
}

func (r *MemoryTutorRepo) Availability(_ context.Context, tutorID string, after time.Time) ([]domain.Slot, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []domain.Slot{}
	for _, s := range r.slots {
		if s.TutorID == tutorID && !s.Booked && s.StartUTC.After(after) {
			out = append(out, *s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartUTC.Before(out[j].StartUTC) })
	return out, nil
}

func (r *MemoryTutorRepo) Slot(_ context.Context, id string) (*domain.Slot, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if s, ok := r.slots[id]; ok {
		cp := *s
		return &cp, nil
	}
	return nil, nil
}

func (r *MemoryTutorRepo) SetSlotBooked(_ context.Context, slotID string, booked bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.slots[slotID]; ok {
		s.Booked = booked
	}
	return nil
}

func (r *MemoryTutorRepo) CreateBooking(_ context.Context, b *domain.Booking) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *b
	r.bookings[b.ID] = &cp
	return nil
}

func (r *MemoryTutorRepo) Booking(_ context.Context, id string) (*domain.Booking, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if b, ok := r.bookings[id]; ok {
		cp := *b
		return &cp, nil
	}
	return nil, nil
}

func (r *MemoryTutorRepo) BookingsByLearner(_ context.Context, learnerID string) ([]domain.Booking, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []domain.Booking{}
	for _, b := range r.bookings {
		if b.LearnerID == learnerID {
			out = append(out, *b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartUTC.After(out[j].StartUTC) })
	return out, nil
}

func (r *MemoryTutorRepo) UpdateBookingStatus(_ context.Context, id, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if b, ok := r.bookings[id]; ok {
		b.Status = status
	}
	return nil
}

func (r *MemoryTutorRepo) CreateReview(_ context.Context, rv *domain.Review) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *rv
	r.reviews[rv.BookingID] = &cp
	return nil
}

func (r *MemoryTutorRepo) ReviewByBooking(_ context.Context, bookingID string) (*domain.Review, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if rv, ok := r.reviews[bookingID]; ok {
		cp := *rv
		return &cp, nil
	}
	return nil, nil
}

func (r *MemoryTutorRepo) RecomputeRating(_ context.Context, tutorID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	sum, cnt := 0, 0
	for _, rv := range r.reviews {
		if rv.TutorID == tutorID {
			sum += rv.Rating
			cnt++
		}
	}
	if t, ok := r.tutors[tutorID]; ok && cnt > 0 {
		t.RatingAvg = float64(int((float64(sum)/float64(cnt))*10+0.5)) / 10
		t.RatingCount = cnt
	}
	return nil
}

func (r *MemoryTutorRepo) TutorByUser(_ context.Context, userID string) (*domain.Tutor, error) {
	if userID == "" {
		return nil, nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, t := range r.tutors {
		if t.UserID == userID {
			cp := *t
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *MemoryTutorRepo) BookingsByTutor(_ context.Context, tutorID string) ([]domain.Booking, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []domain.Booking{}
	for _, b := range r.bookings {
		if b.TutorID == tutorID {
			out = append(out, *b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartUTC.After(out[j].StartUTC) })
	return out, nil
}

func (r *MemoryTutorRepo) LinkUser(_ context.Context, tutorID, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok := r.tutors[tutorID]; ok {
		t.UserID = userID
	}
	return nil
}

func (r *MemoryTutorRepo) CreateTutor(_ context.Context, t *domain.Tutor) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tutors[t.ID]; !exists {
		r.order = append(r.order, t.ID)
	}
	cp := *t
	r.tutors[t.ID] = &cp
	return nil
}

func (r *MemoryTutorRepo) UpdateTutor(_ context.Context, t *domain.Tutor) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tutors[t.ID]; !ok {
		return apperr.New("tutor.not_found", apperr.KindNotFound)
	}
	cp := *t
	r.tutors[t.ID] = &cp
	return nil
}

func (r *MemoryTutorRepo) DeleteTutor(_ context.Context, tutorID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tutors, tutorID)
	out := r.order[:0]
	for _, id := range r.order {
		if id != tutorID {
			out = append(out, id)
		}
	}
	r.order = out
	return nil
}

func (r *MemoryTutorRepo) BookingsStarting(_ context.Context, from, to time.Time) ([]domain.Booking, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []domain.Booking{}
	for _, b := range r.bookings {
		if b.Status == domain.StatusConfirmed && !b.StartUTC.Before(from) && !b.StartUTC.After(to) {
			out = append(out, *b)
		}
	}
	return out, nil
}

func (r *MemoryTutorRepo) SetStatus(_ context.Context, tutorID, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok := r.tutors[tutorID]; ok {
		t.Status = status
	}
	return nil
}

func (r *MemoryTutorRepo) AllBookings(_ context.Context) ([]domain.Booking, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []domain.Booking{}
	for _, b := range r.bookings {
		out = append(out, *b)
	}
	return out, nil
}

func (r *MemoryTutorRepo) AddSlot(_ context.Context, s *domain.Slot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *s
	r.slots[s.ID] = &cp
	return nil
}

func (r *MemoryTutorRepo) DeleteSlot(_ context.Context, slotID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.slots, slotID)
	return nil
}
