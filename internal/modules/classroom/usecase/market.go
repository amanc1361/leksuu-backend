package usecase

import (
	"context"
	"fmt"

	"github.com/leksuu/platform/internal/modules/classroom/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/id"
)

// Group classes are sold per seat at the teacher's price: from class credit
// (shared with 1:1 sessions) or at the payment gateway, with the seat held
// while paying. Learners with an old class subscription keep enrolling free.

// Credits is the learner's class credit (kept by the tutor module).
type Credits interface {
	ChargeCredit(ctx context.Context, learnerID string, amount int, reason, ref string) (bool, error)
	RefundCredit(ctx context.Context, learnerID string, amount int, reason, ref string) error
}

// ClassPayments opens a gateway payment for a seat checkout.
type ClassPayments interface {
	StartClassPayment(ctx context.Context, userID, checkoutID string, amountToman int, desc string) (string, error)
}

// SharePercent supplies the teacher's share of a price (0–100).
type SharePercent interface {
	TeacherSharePercent(ctx context.Context) int
}

func (s *Service) SetMarket(m domain.MarketStore) { s.market = m }
func (s *Service) SetCredits(c Credits)           { s.credits = c }
func (s *Service) SetPayments(p ClassPayments)    { s.payments = p }
func (s *Service) SetShare(p SharePercent)        { s.share = p }

var (
	errClassPrice      = apperr.New("classroom.invalid_price", apperr.KindValidation)
	errClassNoPrice    = apperr.New("classroom.price_not_set", apperr.KindValidation)
	errClassPayRequire = apperr.New("classroom.payment_required", apperr.KindForbidden)
	errClassNoPayments = apperr.New("classroom.payments_unavailable", apperr.KindValidation)
	errClassFull       = apperr.New("classroom.full", apperr.KindConflict)
)

func (s *Service) teacherShare(ctx context.Context) int {
	if s.share == nil {
		return 100
	}
	if p := s.share.TeacherSharePercent(ctx); p >= 0 && p <= 100 {
		return p
	}
	return 100
}

func (s *Service) subscribed(ctx context.Context, studentID string) bool {
	return s.classAccess != nil && s.classAccess.RequireClass(ctx, studentID) == nil
}

// sellable loads a class a student can still buy a seat in, with its price.
func (s *Service) sellable(ctx context.Context, studentID, classID string) (*domain.ClassSession, int, error) {
	c, err := s.repo.GetClass(ctx, classID)
	if err != nil {
		return nil, 0, err
	}
	if c == nil {
		return nil, 0, apperr.New("classroom.not_found", apperr.KindNotFound)
	}
	if c.EndUTC().Before(s.now()) {
		return nil, 0, apperr.New("classroom.finished", apperr.KindValidation)
	}
	if already, _ := s.repo.IsEnrolled(ctx, classID, studentID); already {
		return nil, 0, apperr.New("classroom.already_enrolled", apperr.KindConflict)
	}
	price, err := s.market.ClassPrice(ctx, classID)
	if err != nil {
		return nil, 0, err
	}
	if price <= 0 {
		return nil, 0, errClassNoPrice
	}
	if free, err := s.freeSeats(ctx, c); err != nil {
		return nil, 0, err
	} else if free <= 0 {
		return nil, 0, errClassFull
	}
	return c, price, nil
}

// freeSeats: capacity minus enrolled students and seats held by payments in
// progress (a hold lapses after domain.SeatHold).
func (s *Service) freeSeats(ctx context.Context, c *domain.ClassSession) (int, error) {
	n, err := s.repo.EnrollmentCount(ctx, c.ID)
	if err != nil {
		return 0, err
	}
	held, err := s.market.HeldSeats(ctx, c.ID, s.now().Add(-domain.SeatHold))
	if err != nil {
		return 0, err
	}
	return c.Capacity - n - held, nil
}

// Enroll takes a seat: free with an old class subscription; otherwise paid from
// class credit, or classroom.payment_required tells the client to pay at the
// gateway (CheckoutSeat).
func (s *Service) Enroll(ctx context.Context, studentID, classID string) error {
	if s.market == nil || s.subscribed(ctx, studentID) {
		return s.enrollLegacy(ctx, studentID, classID)
	}
	c, price, err := s.sellable(ctx, studentID, classID)
	if err != nil {
		return err
	}
	if s.credits == nil {
		return errClassPayRequire
	}
	ok, err := s.credits.ChargeCredit(ctx, studentID, price, "group_class", classID)
	if err != nil {
		return err
	}
	if !ok {
		return errClassPayRequire
	}
	if err := s.seat(ctx, c, studentID, price, domain.PaidViaCredit); err != nil {
		_ = s.credits.RefundCredit(ctx, studentID, price, "enroll_failed", classID)
		return err
	}
	return nil
}

// seat enrolls the student and records what the seat cost.
func (s *Service) seat(ctx context.Context, c *domain.ClassSession, studentID string, price int, paidVia string) error {
	if err := s.repo.Enroll(ctx, c.ID, studentID); err != nil {
		return err
	}
	if err := s.market.SaveEnrollmentPrice(ctx, domain.EnrollmentPrice{
		ClassID: c.ID, StudentID: studentID, PriceToman: price, SharePercent: s.teacherShare(ctx), PaidVia: paidVia,
	}); err != nil {
		return err
	}
	if s.notif != nil {
		sn := s.names(ctx, studentID)[studentID]
		_ = s.notif.Notify(ctx, c.TutorID, "class_enrolled", map[string]string{"student_name": sn, "class_title": c.Title}, "/teach")
	}
	return nil
}

// CheckoutSeat holds a seat and opens a gateway payment for it.
func (s *Service) CheckoutSeat(ctx context.Context, studentID, classID string) (string, error) {
	if s.market == nil || s.payments == nil {
		return "", errClassNoPayments
	}
	c, price, err := s.sellable(ctx, studentID, classID)
	if err != nil {
		return "", err
	}
	co := domain.ClassCheckout{ID: id.New(), ClassID: c.ID, StudentID: studentID, PriceToman: price, Status: domain.CheckoutPending, CreatedAt: s.now()}
	if err := s.market.CreateCheckout(ctx, co); err != nil {
		return "", err
	}
	url, err := s.payments.StartClassPayment(ctx, studentID, co.ID, price, fmt.Sprintf("Leksuu class %s", c.Title))
	if err != nil {
		_ = s.market.SetCheckout(ctx, co.ID, domain.CheckoutFailed)
		return "", err
	}
	return url, nil
}

// FulfillCheckout settles a seat payment (safe to repeat). A failed payment
// frees the held seat. A paid seat is taken if the class still has room;
// otherwise (class full or over) the money becomes class credit.
func (s *Service) FulfillCheckout(ctx context.Context, checkoutID string, paid bool) error {
	if s.market == nil {
		return errClassNoPayments
	}
	co, err := s.market.Checkout(ctx, checkoutID)
	if err != nil {
		return err
	}
	if co == nil {
		return apperr.New("classroom.checkout_not_found", apperr.KindNotFound)
	}
	if co.Status == domain.CheckoutPaid || co.Status == domain.CheckoutFailed {
		return nil
	}
	if !paid {
		return s.market.SetCheckout(ctx, co.ID, domain.CheckoutFailed)
	}
	// Our own hold no longer counts once we settle it.
	if err := s.market.SetCheckout(ctx, co.ID, domain.CheckoutPaid); err != nil {
		return err
	}
	c, err := s.repo.GetClass(ctx, co.ClassID)
	if err != nil {
		return err
	}
	refund := c == nil || c.EndUTC().Before(s.now())
	if !refund {
		if already, _ := s.repo.IsEnrolled(ctx, c.ID, co.StudentID); already {
			refund = true
		} else if free, err := s.freeSeats(ctx, c); err != nil {
			return err
		} else if free <= 0 {
			refund = true
		}
	}
	if refund {
		if s.credits == nil {
			return errClassNoPayments
		}
		return s.credits.RefundCredit(ctx, co.StudentID, co.PriceToman, "seat_unavailable", co.ClassID)
	}
	return s.seat(ctx, c, co.StudentID, co.PriceToman, domain.PaidViaPayment)
}

// refundSeat returns a paid seat's price as credit when the student leaves
// more than 24h before the class (once).
func (s *Service) refundSeat(ctx context.Context, studentID, classID string) error {
	if s.market == nil || s.credits == nil {
		return nil
	}
	p, err := s.market.EnrollmentPrice(ctx, classID, studentID)
	if err != nil || p == nil || p.RefundedToman > 0 || p.PriceToman == 0 {
		return err
	}
	c, err := s.repo.GetClass(ctx, classID)
	if err != nil || c == nil {
		return err
	}
	if c.StartUTC.Sub(s.now()) <= domain.RefundWindow {
		return nil // late: no refund, the teacher keeps their share
	}
	if err := s.credits.RefundCredit(ctx, studentID, p.PriceToman, "refund_left_class", classID); err != nil {
		return err
	}
	return s.market.SetEnrollmentRefunded(ctx, classID, studentID, p.PriceToman)
}

// withMoney adds each class's seat price, revenue and the teacher's earned
// share (paid seats not refunded, including late leavers).
func (s *Service) withMoney(ctx context.Context, views []ClassView) []ClassView {
	if s.market == nil || len(views) == 0 {
		return views
	}
	ids := make([]string, 0, len(views))
	for _, v := range views {
		ids = append(ids, v.ID)
	}
	seats, _ := s.market.EnrollmentPrices(ctx, ids)
	for i := range views {
		views[i].PriceToman, _ = s.market.ClassPrice(ctx, views[i].ID)
		for _, p := range seats[views[i].ID] {
			if p.RefundedToman == 0 {
				views[i].RevenueToman += p.PriceToman
				views[i].TeacherEarnedToman += p.TeacherAmount()
			}
		}
	}
	return views
}
