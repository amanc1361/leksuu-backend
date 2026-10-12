package domain

import (
	"context"
	"time"
)

// Group classes are sold per seat at the teacher's price (see tutor sessions).
const (
	PaidViaCredit  = "credit"
	PaidViaPayment = "payment"

	RefundWindow = 24 * time.Hour   // leave earlier than this → refunded as credit
	SeatHold     = 20 * time.Minute // a seat being paid for is held this long

	CheckoutPending = "pending"
	CheckoutPaid    = "paid"
	CheckoutFailed  = "failed"
	CheckoutExpired = "expired"
)

// EnrollmentPrice is what one seat cost and how the money splits.
type EnrollmentPrice struct {
	ClassID       string
	StudentID     string
	PriceToman    int
	SharePercent  int
	PaidVia       string
	RefundedToman int
}

// TeacherAmount is the teacher's part of the seat price.
func (p EnrollmentPrice) TeacherAmount() int { return p.PriceToman * p.SharePercent / 100 }

// ClassCheckout is a seat being paid at the gateway.
type ClassCheckout struct {
	ID         string
	ClassID    string
	StudentID  string
	PriceToman int
	Status     string
	CreatedAt  time.Time
}

// MarketStore persists class prices, paid seats and seat checkouts.
type MarketStore interface {
	ClassPrice(ctx context.Context, classID string) (int, error)
	SetClassPrice(ctx context.Context, classID string, priceToman int) error

	SaveEnrollmentPrice(ctx context.Context, p EnrollmentPrice) error
	EnrollmentPrice(ctx context.Context, classID, studentID string) (*EnrollmentPrice, error)
	EnrollmentPrices(ctx context.Context, classIDs []string) (map[string][]EnrollmentPrice, error)
	SetEnrollmentRefunded(ctx context.Context, classID, studentID string, refundedToman int) error

	CreateCheckout(ctx context.Context, c ClassCheckout) error
	Checkout(ctx context.Context, id string) (*ClassCheckout, error)
	// HeldSeats counts pending checkouts for the class started after since.
	HeldSeats(ctx context.Context, classID string, since time.Time) (int, error)
	SetCheckout(ctx context.Context, id, status string) error
}
