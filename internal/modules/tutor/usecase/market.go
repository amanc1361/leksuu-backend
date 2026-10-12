package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/leksuu/platform/internal/modules/tutor/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/id"
)

// Online classes are sold per session at the teacher's own price. The learner
// pays from class credit when it covers the price, otherwise at the payment
// gateway; the platform keeps a share set in admin settings. Learners with an
// old class subscription keep booking as before until it ends.

// SharePercent supplies the teacher's share of a session price (0–100).
type SharePercent interface {
	TeacherSharePercent(ctx context.Context) int
}

// PaymentStarter opens a gateway payment for a session checkout and returns
// the URL the learner pays at.
type PaymentStarter interface {
	StartSessionPayment(ctx context.Context, userID, checkoutID string, amountToman int, desc string) (string, error)
}

func (s *Service) SetMarket(m domain.MarketStore) { s.market = m }
func (s *Service) SetShare(p SharePercent)        { s.share = p }
func (s *Service) SetPayments(p PaymentStarter)   { s.payments = p }

var (
	errPriceNotSet     = apperr.New("tutor.price_not_set", apperr.KindValidation)
	errPaymentRequired = apperr.New("tutor.payment_required", apperr.KindForbidden)
	errNoPayments      = apperr.New("tutor.payments_unavailable", apperr.KindValidation)
	errSlotTaken       = apperr.New("tutor.slot_taken", apperr.KindConflict)
)

func (s *Service) sharePct(ctx context.Context) int {
	if s.share == nil {
		return 100
	}
	p := s.share.TeacherSharePercent(ctx)
	if p < 0 || p > 100 {
		return 100
	}
	return p
}

// subscribed reports whether the learner still has an old class subscription.
func (s *Service) subscribed(ctx context.Context, learnerID string) bool {
	return s.classAccess != nil && s.classAccess.RequireClass(ctx, learnerID) == nil
}

// openSlot loads a slot that can still be sold: it exists, starts in the
// future, and — if a checkout held it but was abandoned — the hold is released.
func (s *Service) openSlot(ctx context.Context, slotID string) (*domain.Slot, error) {
	slot, err := s.repo.Slot(ctx, slotID)
	if err != nil {
		return nil, err
	}
	if slot == nil {
		return nil, apperr.New("tutor.slot_not_found", apperr.KindNotFound)
	}
	if !slot.StartUTC.After(s.now()) {
		return nil, apperr.New("tutor.slot_past", apperr.KindValidation)
	}
	if slot.Booked && s.market != nil {
		if c, err := s.market.PendingCheckoutForSlot(ctx, slot.ID); err == nil && c != nil && s.now().Sub(c.CreatedAt) > domain.CheckoutHold {
			_ = s.market.SetCheckout(ctx, c.ID, domain.CheckoutExpired, "")
			if err := s.repo.SetSlotBooked(ctx, slot.ID, false); err != nil {
				return nil, err
			}
			slot.Booked = false
		}
	}
	if slot.Booked {
		return nil, errSlotTaken
	}
	return slot, nil
}

func (s *Service) priceOf(ctx context.Context, tutorID string) (int, error) {
	price, err := s.market.TutorPrice(ctx, tutorID)
	if err != nil {
		return 0, err
	}
	if price <= 0 {
		return 0, errPriceNotSet
	}
	return price, nil
}

// Book reserves a session. An old class subscription books for free; otherwise
// the price is taken from class credit, or tutor.payment_required tells the
// client to pay at the gateway (CheckoutSession).
func (s *Service) Book(ctx context.Context, learnerID, slotID string) (*domain.Booking, error) {
	if s.market == nil || s.subscribed(ctx, learnerID) {
		return s.bookLegacy(ctx, learnerID, slotID)
	}
	slot, err := s.openSlot(ctx, slotID)
	if err != nil {
		return nil, err
	}
	price, err := s.priceOf(ctx, slot.TutorID)
	if err != nil {
		return nil, err
	}
	if bal, err := s.market.CreditBalance(ctx, learnerID); err != nil {
		return nil, err
	} else if bal < price {
		return nil, errPaymentRequired
	}
	if ok, err := s.market.ClaimSlot(ctx, slot.ID); err != nil {
		return nil, err
	} else if !ok {
		return nil, errSlotTaken
	}
	bookingID := id.New()
	if _, err := s.market.AdjustCredit(ctx, learnerID, -price, "session", bookingID); err != nil {
		_ = s.repo.SetSlotBooked(ctx, slot.ID, false)
		if errors.Is(err, domain.ErrInsufficientCredit) {
			return nil, errPaymentRequired
		}
		return nil, err
	}
	b, err := s.confirmSession(ctx, learnerID, slot, bookingID, price, domain.PaidViaCredit)
	if err != nil {
		_, _ = s.market.AdjustCredit(ctx, learnerID, price, "booking_failed", bookingID)
		_ = s.repo.SetSlotBooked(ctx, slot.ID, false)
		return nil, err
	}
	return b, nil
}

// confirmSession creates the booking for an already-claimed slot and records
// its price and the teacher's share at this moment.
func (s *Service) confirmSession(ctx context.Context, learnerID string, slot *domain.Slot, bookingID string, price int, paidVia string) (*domain.Booking, error) {
	b := &domain.Booking{
		ID: bookingID, TutorID: slot.TutorID, LearnerID: learnerID, SlotID: slot.ID,
		StartUTC: slot.StartUTC, DurationMin: slot.DurationMin, Status: domain.StatusConfirmed,
		RoomID: "room_" + id.New(), CreatedAt: s.now(),
	}
	if err := s.repo.CreateBooking(ctx, b); err != nil {
		return nil, err
	}
	if err := s.market.SaveBookingPrice(ctx, domain.BookingPrice{
		BookingID: b.ID, PriceToman: price, SharePercent: s.sharePct(ctx), PaidVia: paidVia,
	}); err != nil {
		return nil, err
	}
	s.notifyTutor(ctx, b.TutorID, domain.NotifBookingNew, map[string]string{
		"learner_name": s.learnerName(ctx, learnerID), "when": b.StartUTC.Format(time.RFC3339),
	})
	return b, nil
}

// CheckoutSession holds the slot and opens a gateway payment for it; the
// booking is created when the payment is confirmed (FulfillCheckout).
func (s *Service) CheckoutSession(ctx context.Context, learnerID, slotID string) (string, error) {
	if s.market == nil || s.payments == nil {
		return "", errNoPayments
	}
	slot, err := s.openSlot(ctx, slotID)
	if err != nil {
		return "", err
	}
	price, err := s.priceOf(ctx, slot.TutorID)
	if err != nil {
		return "", err
	}
	if ok, err := s.market.ClaimSlot(ctx, slot.ID); err != nil {
		return "", err
	} else if !ok {
		return "", errSlotTaken
	}
	c := domain.Checkout{ID: id.New(), LearnerID: learnerID, SlotID: slot.ID, PriceToman: price, Status: domain.CheckoutPending, CreatedAt: s.now()}
	if err := s.market.CreateCheckout(ctx, c); err != nil {
		_ = s.repo.SetSlotBooked(ctx, slot.ID, false)
		return "", err
	}
	desc := fmt.Sprintf("Leksuu session %s", slot.StartUTC.Format("2006-01-02 15:04"))
	url, err := s.payments.StartSessionPayment(ctx, learnerID, c.ID, price, desc)
	if err != nil {
		_ = s.market.SetCheckout(ctx, c.ID, domain.CheckoutFailed, "")
		_ = s.repo.SetSlotBooked(ctx, slot.ID, false)
		return "", err
	}
	return url, nil
}

// FulfillCheckout settles a gateway payment for a session. Safe to call more
// than once. A failed payment releases the slot; a payment that arrives after
// the hold expired (slot gone) becomes class credit, so nothing is lost.
func (s *Service) FulfillCheckout(ctx context.Context, checkoutID string, paid bool) error {
	if s.market == nil {
		return errNoPayments
	}
	c, err := s.market.Checkout(ctx, checkoutID)
	if err != nil {
		return err
	}
	if c == nil {
		return apperr.New("tutor.checkout_not_found", apperr.KindNotFound)
	}
	switch c.Status {
	case domain.CheckoutPaid, domain.CheckoutFailed:
		return nil
	case domain.CheckoutExpired:
		if paid {
			if _, err := s.market.AdjustCredit(ctx, c.LearnerID, c.PriceToman, "payment_after_hold_expired", ""); err != nil {
				return err
			}
			return s.market.SetCheckout(ctx, c.ID, domain.CheckoutPaid, "")
		}
		return nil
	}
	if !paid {
		if err := s.market.SetCheckout(ctx, c.ID, domain.CheckoutFailed, ""); err != nil {
			return err
		}
		return s.repo.SetSlotBooked(ctx, c.SlotID, false)
	}
	slot, err := s.repo.Slot(ctx, c.SlotID)
	if err != nil {
		return err
	}
	if slot == nil { // the teacher removed the slot meanwhile: refund as credit
		if _, err := s.market.AdjustCredit(ctx, c.LearnerID, c.PriceToman, "slot_removed", ""); err != nil {
			return err
		}
		return s.market.SetCheckout(ctx, c.ID, domain.CheckoutPaid, "")
	}
	b, err := s.confirmSession(ctx, c.LearnerID, slot, id.New(), c.PriceToman, domain.PaidViaPayment)
	if err != nil {
		return err
	}
	return s.market.SetCheckout(ctx, c.ID, domain.CheckoutPaid, b.ID)
}

// refund returns a cancelled session's price to the learner as credit: always
// when the teacher cancels; when the learner cancels, only more than 24h
// ahead. Subscription bookings carry no price and are not refunded.
func (s *Service) refund(ctx context.Context, b *domain.Booking, teacherCancelled bool) error {
	if s.market == nil {
		return nil
	}
	prices, err := s.market.BookingPrices(ctx, []string{b.ID})
	if err != nil {
		return err
	}
	p, ok := prices[b.ID]
	if !ok || p.RefundedToman > 0 || p.PriceToman == 0 {
		return nil
	}
	if !teacherCancelled && b.StartUTC.Sub(s.now()) <= domain.RefundWindow {
		return nil // late cancellation: no refund, the teacher keeps their share
	}
	reason := "refund_cancelled"
	if teacherCancelled {
		reason = "refund_teacher_cancelled"
	}
	if _, err := s.market.AdjustCredit(ctx, b.LearnerID, p.PriceToman, reason, b.ID); err != nil {
		return err
	}
	return s.market.SetRefunded(ctx, b.ID, p.PriceToman)
}

// Credit is a learner's class credit balance (Toman).
func (s *Service) Credit(ctx context.Context, learnerID string) (int, error) {
	if s.market == nil {
		return 0, nil
	}
	return s.market.CreditBalance(ctx, learnerID)
}

// SetMyPrice lets a teacher set their price per session (Toman, > 0).
func (s *Service) SetMyPrice(ctx context.Context, userID string, priceToman int) error {
	if s.market == nil {
		return errNoPayments
	}
	if priceToman <= 0 {
		return apperr.New("tutor.invalid_price", apperr.KindValidation)
	}
	t, err := s.TutorForUser(ctx, userID)
	if err != nil {
		return err
	}
	if t == nil {
		return apperr.New("tutor.not_a_tutor", apperr.KindForbidden)
	}
	return s.market.SetTutorPrice(ctx, t.ID, priceToman)
}

// sessionEarning is what a priced session pays the teacher: their share when
// it took place, or when the learner cancelled too late to be refunded.
func sessionEarning(b domain.Booking, p domain.BookingPrice, now time.Time) (earned int, counts bool) {
	switch b.EffectiveStatus(now) {
	case domain.StatusCompleted:
		return p.TeacherAmount(), true
	case domain.StatusCancelled:
		if p.RefundedToman == 0 {
			return p.TeacherAmount(), true
		}
	}
	return 0, false
}

// withPrices fills each tutor's price per session (public profile).
func (s *Service) withPrices(ctx context.Context, ts []domain.Tutor) []domain.Tutor {
	if s.market == nil {
		return ts
	}
	for i := range ts {
		if p, err := s.market.TutorPrice(ctx, ts[i].ID); err == nil {
			ts[i].SessionPriceToman = p
		}
	}
	return ts
}

// MyTutorProfile is the signed-in teacher's profile including their price.
func (s *Service) MyTutorProfile(ctx context.Context, userID string) (*domain.Tutor, error) {
	t, err := s.TutorForUser(ctx, userID)
	if err != nil || t == nil {
		return t, err
	}
	withPrice := s.withPrices(ctx, []domain.Tutor{*t})[0]
	return &withPrice, nil
}

// ChargeCredit takes a price from a learner's class credit (shared by 1:1
// sessions and group classes); false when the balance doesn't cover it.
func (s *Service) ChargeCredit(ctx context.Context, learnerID string, amount int, reason, ref string) (bool, error) {
	if s.market == nil {
		return false, nil
	}
	if _, err := s.market.AdjustCredit(ctx, learnerID, -amount, reason, ref); err != nil {
		if errors.Is(err, domain.ErrInsufficientCredit) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// RefundCredit adds an amount back to a learner's class credit.
func (s *Service) RefundCredit(ctx context.Context, learnerID string, amount int, reason, ref string) error {
	if s.market == nil {
		return errNoPayments
	}
	_, err := s.market.AdjustCredit(ctx, learnerID, amount, reason, ref)
	return err
}
