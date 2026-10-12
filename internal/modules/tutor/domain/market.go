package domain

import (
	"context"
	"errors"
	"time"
)

// Paying for sessions. A teacher sets a price per session; the learner pays it
// (from class credit, or at the payment gateway); the platform keeps a share.
const (
	PaidViaCredit  = "credit"
	PaidViaPayment = "payment"

	// RefundWindow: cancelling earlier than this before the start refunds the
	// learner as credit; later cancellations are not refunded.
	RefundWindow = 24 * time.Hour

	// CheckoutHold: how long a slot stays held for a learner paying at the
	// gateway before another learner may take it.
	CheckoutHold = 20 * time.Minute

	CheckoutPending = "pending"
	CheckoutPaid    = "paid"
	CheckoutFailed  = "failed"
	CheckoutExpired = "expired"
)

// ErrInsufficientCredit: a credit debit would make the balance negative.
var ErrInsufficientCredit = errors.New("insufficient credit")

// BookingPrice is what a booking cost and how the money splits, recorded when
// the booking is made so later price or share changes don't rewrite history.
type BookingPrice struct {
	BookingID     string
	PriceToman    int
	SharePercent  int // the teacher's share at booking time
	PaidVia       string
	RefundedToman int
}

// TeacherAmount is the teacher's part of the price.
func (p BookingPrice) TeacherAmount() int { return p.PriceToman * p.SharePercent / 100 }

// Checkout is a session being paid at the gateway; its slot is held meanwhile.
type Checkout struct {
	ID         string
	LearnerID  string
	SlotID     string
	PriceToman int
	Status     string
	BookingID  string
	CreatedAt  time.Time
}

// MarketStore persists prices, credit and checkouts.
type MarketStore interface {
	TutorPrice(ctx context.Context, tutorID string) (int, error)
	SetTutorPrice(ctx context.Context, tutorID string, priceToman int) error

	// ClaimSlot marks a free slot booked, atomically: false if it was taken.
	ClaimSlot(ctx context.Context, slotID string) (bool, error)

	SaveBookingPrice(ctx context.Context, p BookingPrice) error
	BookingPrices(ctx context.Context, bookingIDs []string) (map[string]BookingPrice, error)
	SetRefunded(ctx context.Context, bookingID string, refundedToman int) error

	CreditBalance(ctx context.Context, userID string) (int, error)
	// AdjustCredit changes a balance and records why; a debit that would go
	// below zero fails with ErrInsufficientCredit and changes nothing.
	AdjustCredit(ctx context.Context, userID string, deltaToman int, reason, bookingID string) (int, error)

	CreateCheckout(ctx context.Context, c Checkout) error
	Checkout(ctx context.Context, id string) (*Checkout, error)
	PendingCheckoutForSlot(ctx context.Context, slotID string) (*Checkout, error)
	SetCheckout(ctx context.Context, id, status, bookingID string) error
}
