// Package domain holds the live-tutoring bounded context: tutor profiles,
// bookable availability slots (stored in UTC for timezone safety), bookings,
// and learner reviews that feed each tutor's rating.
package domain

import (
	"context"
	"time"
)

// Tutor application/approval statuses.
const (
	StatusPending  = "pending"
	StatusApproved = "approved"
	StatusRejected = "rejected"
)

// Notification type strings emitted by this module.
const (
	NotifBookingNew       = "booking_new"
	NotifBookingCancelled = "booking_cancelled"
	NotifTutorCancelled   = "class_cancelled_by_tutor"
	NotifReviewNew        = "review_new"
	NotifTutorWelcome     = "tutor_welcome"
	NotifTutorApproved    = "tutor_approved"
	NotifTutorRejected    = "tutor_rejected"
)

type Tutor struct {
	ID          string   `json:"id"`
	UserID      string   `json:"user_id,omitempty"` // linked user account (a tutor logs in as this user)
	Status      string   `json:"status"`            // pending | approved | rejected
	Name        string   `json:"name"`
	Headline    string   `json:"headline"`
	Bio         string   `json:"bio"`
	Languages   []string `json:"languages"`
	AvatarURL   string   `json:"avatar_url"`
	HourlyGems  int      `json:"hourly_gems"`
	RatingAvg   float64  `json:"rating_avg"`
	RatingCount int      `json:"rating_count"`
	// SessionPriceToman is the teacher's price per session (0 = not set yet).
	SessionPriceToman int `json:"session_price_toman"`
}

// Slot is one bookable opening. StartUTC is always UTC; the client renders it in
// the learner's local timezone.
type Slot struct {
	ID          string    `json:"id"`
	TutorID     string    `json:"tutor_id"`
	StartUTC    time.Time `json:"start_utc"`
	DurationMin int       `json:"duration_min"`
	Booked      bool      `json:"booked"`
}

// Booking statuses persisted in storage. "completed" is derived at read time
// (when a confirmed booking's end is in the past), never stored.
const (
	StatusConfirmed = "confirmed"
	StatusCancelled = "cancelled"
	StatusCompleted = "completed" // derived only
)

type Booking struct {
	ID          string    `json:"id"`
	TutorID     string    `json:"tutor_id"`
	LearnerID   string    `json:"learner_id"`
	SlotID      string    `json:"slot_id"`
	StartUTC    time.Time `json:"start_utc"`
	DurationMin int       `json:"duration_min"`
	Status      string    `json:"status"`
	RoomID      string    `json:"room_id"`
	CreatedAt   time.Time `json:"created_at"`
	CostGems    int       `json:"cost_gems"`
}

func (b Booking) EndUTC() time.Time {
	return b.StartUTC.Add(time.Duration(b.DurationMin) * time.Minute)
}

// EffectiveStatus derives the user-visible status from stored status + time.
func (b Booking) EffectiveStatus(now time.Time) string {
	if b.Status == StatusCancelled {
		return StatusCancelled
	}
	if now.After(b.EndUTC()) {
		return StatusCompleted
	}
	return StatusConfirmed
}

type Review struct {
	ID        string    `json:"id"`
	BookingID string    `json:"booking_id"`
	TutorID   string    `json:"tutor_id"`
	LearnerID string    `json:"learner_id"`
	Rating    int       `json:"rating"`
	Comment   string    `json:"comment"`
	CreatedAt time.Time `json:"created_at"`
}

type Repository interface {
	CreateTutor(ctx context.Context, t *Tutor) error
	UpdateTutor(ctx context.Context, t *Tutor) error
	DeleteTutor(ctx context.Context, tutorID string) error
	ListTutors(ctx context.Context, lang string) ([]Tutor, error)
	Tutor(ctx context.Context, id string) (*Tutor, error)
	Availability(ctx context.Context, tutorID string, after time.Time) ([]Slot, error)
	Slot(ctx context.Context, id string) (*Slot, error)
	SetSlotBooked(ctx context.Context, slotID string, booked bool) error
	// AddSlot inserts a new (unbooked) opening; DeleteSlot removes one.
	// Ownership/booked checks live in the usecase.
	AddSlot(ctx context.Context, s *Slot) error
	DeleteSlot(ctx context.Context, slotID string) error
	CreateBooking(ctx context.Context, b *Booking) error
	Booking(ctx context.Context, id string) (*Booking, error)
	BookingsByLearner(ctx context.Context, learnerID string) ([]Booking, error)
	UpdateBookingStatus(ctx context.Context, id, status string) error
	CreateReview(ctx context.Context, r *Review) error
	ReviewByBooking(ctx context.Context, bookingID string) (*Review, error)
	RecomputeRating(ctx context.Context, tutorID string) error
	TutorByUser(ctx context.Context, userID string) (*Tutor, error)
	BookingsByTutor(ctx context.Context, tutorID string) ([]Booking, error)
	AllBookings(ctx context.Context) ([]Booking, error)
	BookingsStarting(ctx context.Context, from, to time.Time) ([]Booking, error)
	LinkUser(ctx context.Context, tutorID, userID string) error
	SetStatus(ctx context.Context, tutorID, status string) error
}
