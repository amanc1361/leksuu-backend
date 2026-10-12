package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/leksuu/platform/internal/modules/classroom/domain"
	"github.com/leksuu/platform/internal/modules/classroom/repo"
	"github.com/leksuu/platform/internal/shared/apperr"
)

// fakeCredits is a class-credit ledger (the real one lives in the tutor module).
type fakeCredits map[string]int

func (f fakeCredits) ChargeCredit(_ context.Context, u string, amt int, _, _ string) (bool, error) {
	if f[u] < amt {
		return false, nil
	}
	f[u] -= amt
	return true, nil
}
func (f fakeCredits) RefundCredit(_ context.Context, u string, amt int, _, _ string) error {
	f[u] += amt
	return nil
}

type fakeClassPay struct{ last string }

func (f *fakeClassPay) StartClassPayment(_ context.Context, _, checkoutID string, _ int, _ string) (string, error) {
	f.last = checkoutID
	return "https://pay.example/" + checkoutID, nil
}

type pct int

func (p pct) TeacherSharePercent(context.Context) int { return int(p) }

type allowAll struct{}

func (allowAll) RequireClass(context.Context, string) error { return nil }

const seatPrice = 150000

type seats struct {
	svc     *Service
	r       *repo.MemoryClassRepo
	now     *time.Time
	credits fakeCredits
	pay     *fakeClassPay
}

func newSeats(t *testing.T) seats {
	t.Helper()
	now := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	s := seats{r: repo.NewMemoryClassRepo(), now: &now, credits: fakeCredits{}, pay: &fakeClassPay{}}
	s.svc = NewService(s.r, fakeTutors{"tutor-1": true}, nil, func() time.Time { return *s.now })
	s.svc.SetMarket(s.r)
	s.svc.SetCredits(s.credits)
	s.svc.SetPayments(s.pay)
	s.svc.SetShare(pct(70))
	return s
}

func (s seats) class(t *testing.T, in time.Duration, capacity int) string {
	t.Helper()
	c, err := s.svc.Create(context.Background(), "tutor-1", CreateInput{Title: "Talk", StartUTC: s.now.Add(in), DurationMin: 60, Capacity: capacity, PriceToman: seatPrice})
	if err != nil {
		t.Fatal(err)
	}
	return c.ID
}

func code(err error) string {
	if e, ok := err.(*apperr.Error); ok {
		return e.Code
	}
	return ""
}

func TestPaidClassNeedsAPrice(t *testing.T) {
	s := newSeats(t)
	_, err := s.svc.Create(context.Background(), "tutor-1", CreateInput{Title: "x", StartUTC: s.now.Add(48 * time.Hour), DurationMin: 60, Capacity: 4})
	if code(err) != "classroom.invalid_price" {
		t.Fatalf("want classroom.invalid_price, got %v", err)
	}
}

// Without credit a seat is paid at the gateway; the held seat counts against
// capacity, and the confirmed payment enrolls once.
func TestSeatPaidAtGateway(t *testing.T) {
	ctx := context.Background()
	s := newSeats(t)
	cls := s.class(t, 48*time.Hour, 1)
	if err := s.svc.Enroll(ctx, "A", cls); code(err) != "classroom.payment_required" {
		t.Fatalf("no credit: want classroom.payment_required, got %v", err)
	}
	if _, err := s.svc.CheckoutSeat(ctx, "A", cls); err != nil {
		t.Fatal(err)
	}
	if _, err := s.svc.CheckoutSeat(ctx, "B", cls); code(err) != "classroom.full" {
		t.Fatalf("the only seat is held: want classroom.full, got %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := s.svc.FulfillCheckout(ctx, s.pay.last, true); err != nil {
			t.Fatal(err)
		}
	}
	if ok, _ := s.r.IsEnrolled(ctx, cls, "A"); !ok {
		t.Fatal("A should be enrolled after paying")
	}
	if n, _ := s.r.EnrollmentCount(ctx, cls); n != 1 {
		t.Fatalf("enrolled %d, want 1", n)
	}
	if p, _ := s.r.EnrollmentPrice(ctx, cls, "A"); p == nil || p.PriceToman != seatPrice || p.SharePercent != 70 || p.PaidVia != domain.PaidViaPayment {
		t.Fatalf("seat price = %+v", p)
	}
}

// A hold lapses after 20 minutes; if that payment still arrives and the
// class has filled meanwhile, the money becomes credit.
func TestLapsedHoldLatePaymentBecomesCredit(t *testing.T) {
	ctx := context.Background()
	s := newSeats(t)
	cls := s.class(t, 48*time.Hour, 1)
	_, _ = s.svc.CheckoutSeat(ctx, "A", cls)
	first := s.pay.last
	*s.now = s.now.Add(domain.SeatHold + time.Minute)
	if _, err := s.svc.CheckoutSeat(ctx, "B", cls); err != nil {
		t.Fatalf("lapsed hold should free the seat: %v", err)
	}
	if err := s.svc.FulfillCheckout(ctx, s.pay.last, true); err != nil { // B pays first
		t.Fatal(err)
	}
	if err := s.svc.FulfillCheckout(ctx, first, true); err != nil { // A pays late
		t.Fatal(err)
	}
	if ok, _ := s.r.IsEnrolled(ctx, cls, "A"); ok || s.credits["A"] != seatPrice {
		t.Fatalf("late payer: enrolled=%v credit=%d (want not enrolled, full credit)", ok, s.credits["A"])
	}
}

// Credit pays for a seat; leaving >24h ahead refunds once, later doesn't.
func TestSeatCreditAndLeaveRefunds(t *testing.T) {
	ctx := context.Background()
	s := newSeats(t)
	s.credits["A"] = 2 * seatPrice
	far, near := s.class(t, 48*time.Hour, 4), s.class(t, 3*time.Hour, 4)
	if err := s.svc.Enroll(ctx, "A", far); err != nil {
		t.Fatal(err)
	}
	if err := s.svc.Enroll(ctx, "A", near); err != nil || s.credits["A"] != 0 {
		t.Fatalf("two seats from credit: credit %d err %v", s.credits["A"], err)
	}
	if err := s.svc.Leave(ctx, "A", far); err != nil || s.credits["A"] != seatPrice {
		t.Fatalf(">24h: refund expected, credit %d err %v", s.credits["A"], err)
	}
	if err := s.svc.Leave(ctx, "A", near); err != nil || s.credits["A"] != seatPrice {
		t.Fatalf("<24h: no refund, credit %d err %v", s.credits["A"], err)
	}
	_ = s.svc.Leave(ctx, "A", far)
	if s.credits["A"] != seatPrice {
		t.Fatalf("double refund: credit %d", s.credits["A"])
	}
	// Teacher's view: the late leaver's seat still counts, the refunded one not.
	views, _ := s.svc.TutorClasses(ctx, "tutor-1")
	got := map[string]ClassView{}
	for _, v := range views {
		got[v.ID] = v
	}
	if got[near].RevenueToman != seatPrice || got[near].TeacherEarnedToman != seatPrice*70/100 || got[far].RevenueToman != 0 || got[near].PriceToman != seatPrice {
		t.Fatalf("money views: near=%+v far=%+v", got[near], got[far])
	}
}

// Learners with an old class subscription keep enrolling for free.
func TestClassSubscriberEnrollsFree(t *testing.T) {
	ctx := context.Background()
	s := newSeats(t)
	s.svc.SetClassAccess(allowAll{})
	cls := s.class(t, 48*time.Hour, 4)
	if err := s.svc.Enroll(ctx, "A", cls); err != nil {
		t.Fatalf("subscriber: %v", err)
	}
	if p, _ := s.r.EnrollmentPrice(ctx, cls, "A"); p != nil {
		t.Fatal("a subscription seat carries no price")
	}
}
