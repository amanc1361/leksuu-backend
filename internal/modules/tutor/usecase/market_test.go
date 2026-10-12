package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/leksuu/platform/internal/modules/tutor/domain"
	"github.com/leksuu/platform/internal/modules/tutor/repo"
	"github.com/leksuu/platform/internal/shared/apperr"
)

type shareOf int

func (s shareOf) TeacherSharePercent(context.Context) int { return int(s) }

type fakePay struct {
	calls  int
	amount int
	last   string // checkout id
}

func (f *fakePay) StartSessionPayment(_ context.Context, _, checkoutID string, amount int, _ string) (string, error) {
	f.calls++
	f.amount, f.last = amount, checkoutID
	return "https://pay.example/" + checkoutID, nil
}

const price = 200000

type market struct {
	svc *Service
	r   *repo.MemoryTutorRepo
	now *time.Time
	pay *fakePay
}

func newMarket(t *testing.T) market {
	t.Helper()
	now := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	r := repo.NewMemoryTutorRepo()
	m := market{r: r, now: &now, pay: &fakePay{}}
	m.svc = NewService(r, func() time.Time { return *m.now })
	m.svc.SetMarket(r)
	m.svc.SetShare(shareOf(70))
	m.svc.SetPayments(m.pay)
	if err := r.SetTutorPrice(context.Background(), "tutor-sara", price); err != nil {
		t.Fatal(err)
	}
	return m
}

func (m market) slot(t *testing.T, tutorID string, in time.Duration) string {
	t.Helper()
	s := &domain.Slot{ID: "slot-" + in.String() + "-" + tutorID, TutorID: tutorID, StartUTC: m.now.Add(in), DurationMin: 30}
	if err := m.r.AddSlot(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	return s.ID
}

func codeOf(err error) string {
	if e, ok := err.(*apperr.Error); ok {
		return e.Code
	}
	return ""
}

// Without credit the learner pays at the gateway: the slot is held meanwhile,
// and the confirmed payment books it — once, even if the callback repeats.
func TestSessionPaidAtGatewayIsBookedOnce(t *testing.T) {
	ctx := context.Background()
	m := newMarket(t)
	slot := m.slot(t, "tutor-sara", 48*time.Hour)
	if _, err := m.svc.Book(ctx, "L1", slot); codeOf(err) != "tutor.payment_required" {
		t.Fatalf("no credit: want tutor.payment_required, got %v", err)
	}
	url, err := m.svc.CheckoutSession(ctx, "L1", slot)
	if err != nil || url == "" || m.pay.amount != price {
		t.Fatalf("checkout: url=%q amount=%d err=%v", url, m.pay.amount, err)
	}
	if _, err := m.svc.CheckoutSession(ctx, "L2", slot); codeOf(err) != "tutor.slot_taken" {
		t.Fatalf("held slot must not be sold twice, got %v", err)
	}
	for i := 0; i < 2; i++ { // the gateway callback may arrive twice
		if err := m.svc.FulfillCheckout(ctx, m.pay.last, true); err != nil {
			t.Fatal(err)
		}
	}
	bs, _ := m.svc.MyBookings(ctx, "L1")
	if len(bs) != 1 {
		t.Fatalf("want exactly one booking, got %d", len(bs))
	}
	p, _ := m.r.BookingPrices(ctx, []string{bs[0].ID})
	if got := p[bs[0].ID]; got.PriceToman != price || got.SharePercent != 70 || got.PaidVia != domain.PaidViaPayment {
		t.Fatalf("booking price = %+v", got)
	}
}

// A failed payment frees the slot for others.
func TestFailedPaymentReleasesSlot(t *testing.T) {
	ctx := context.Background()
	m := newMarket(t)
	slot := m.slot(t, "tutor-sara", 48*time.Hour)
	if _, err := m.svc.CheckoutSession(ctx, "L1", slot); err != nil {
		t.Fatal(err)
	}
	if err := m.svc.FulfillCheckout(ctx, m.pay.last, false); err != nil {
		t.Fatal(err)
	}
	if _, err := m.svc.CheckoutSession(ctx, "L2", slot); err != nil {
		t.Fatalf("slot should be free again: %v", err)
	}
}

// An abandoned checkout stops holding the slot after 20 minutes; if its
// payment still arrives later, the money becomes the learner's credit.
func TestAbandonedHoldExpiresAndLatePaymentBecomesCredit(t *testing.T) {
	ctx := context.Background()
	m := newMarket(t)
	slot := m.slot(t, "tutor-sara", 48*time.Hour)
	if _, err := m.svc.CheckoutSession(ctx, "L1", slot); err != nil {
		t.Fatal(err)
	}
	first := m.pay.last
	*m.now = m.now.Add(domain.CheckoutHold + time.Minute)
	if _, err := m.svc.CheckoutSession(ctx, "L2", slot); err != nil {
		t.Fatalf("expired hold should free the slot: %v", err)
	}
	if err := m.svc.FulfillCheckout(ctx, first, true); err != nil {
		t.Fatal(err)
	}
	if bal, _ := m.svc.Credit(ctx, "L1"); bal != price {
		t.Fatalf("late payment should become credit, balance %d", bal)
	}
	if bs, _ := m.svc.MyBookings(ctx, "L1"); len(bs) != 0 {
		t.Fatal("L1 must not also get the slot")
	}
}

// Credit pays for a session; refunds follow the 24h rule; a teacher's
// cancellation always refunds.
func TestCreditBookingAndRefundRules(t *testing.T) {
	ctx := context.Background()
	m := newMarket(t)
	if _, err := m.r.AdjustCredit(ctx, "L1", 3*price, "test", ""); err != nil {
		t.Fatal(err)
	}
	balance := func() int { b, _ := m.svc.Credit(ctx, "L1"); return b }
	early, _ := m.svc.Book(ctx, "L1", m.slot(t, "tutor-sara", 48*time.Hour))
	late, _ := m.svc.Book(ctx, "L1", m.slot(t, "tutor-sara", 2*time.Hour))
	byTeacher, err := m.svc.Book(ctx, "L1", m.slot(t, "tutor-sara", 3*time.Hour))
	if err != nil || balance() != 0 {
		t.Fatalf("three sessions from credit: balance %d err %v", balance(), err)
	}
	if err := m.svc.Cancel(ctx, "L1", early.ID); err != nil || balance() != price {
		t.Fatalf(">24h ahead should refund: balance %d err %v", balance(), err)
	}
	if err := m.svc.Cancel(ctx, "L1", late.ID); err != nil || balance() != price {
		t.Fatalf("<24h ahead must not refund: balance %d err %v", balance(), err)
	}
	if err := m.svc.LinkUser(ctx, "tutor-sara", "sara-user"); err != nil {
		t.Fatal(err)
	}
	if err := m.svc.TutorCancel(ctx, "sara-user", byTeacher.ID); err != nil || balance() != 2*price {
		t.Fatalf("teacher cancellation always refunds: balance %d err %v", balance(), err)
	}
	// Refunding twice is impossible.
	_ = m.svc.Cancel(ctx, "L1", early.ID)
	if balance() != 2*price {
		t.Fatalf("double refund: balance %d", balance())
	}
}

// The teacher earns their share of a priced session when it takes place or
// when the learner cancels too late; the platform keeps the rest.
func TestTeacherEarnsShareOfPrice(t *testing.T) {
	ctx := context.Background()
	m := newMarket(t)
	_, _ = m.r.AdjustCredit(ctx, "L1", 3*price, "test", "")
	_ = m.svc.LinkUser(ctx, "tutor-sara", "sara-user")
	taught, _ := m.svc.Book(ctx, "L1", m.slot(t, "tutor-sara", time.Hour))
	late, _ := m.svc.Book(ctx, "L1", m.slot(t, "tutor-sara", 2*time.Hour))
	early, _ := m.svc.Book(ctx, "L1", m.slot(t, "tutor-sara", 72*time.Hour))
	_ = m.svc.Cancel(ctx, "L1", late.ID)  // <24h: teacher keeps their share
	_ = m.svc.Cancel(ctx, "L1", early.ID) // refunded: nothing earned
	*m.now = taught.EndUTC().Add(time.Minute)
	rep, err := m.svc.EarningsReport(ctx, "sara-user")
	if err != nil {
		t.Fatal(err)
	}
	want := 2 * price * 70 / 100
	if rep.SessionEarningsToman != want || rep.EarningsToman != want {
		t.Fatalf("earnings: session=%d total=%d, want %d", rep.SessionEarningsToman, rep.EarningsToman, want)
	}
	sum, _ := m.svc.TotalPayout(ctx)
	if sum.SessionRevenueToman != 2*price || sum.SessionPayoutToman != want || sum.PlatformShareToman != 2*price-want {
		t.Fatalf("payout summary = %+v", sum)
	}
}

// A learner with an old class subscription keeps booking for free.
func TestSubscriberStillBooksWithoutPaying(t *testing.T) {
	ctx := context.Background()
	m := newMarket(t)
	m.svc.SetClassAccess(fakeClassAccess{allow: true})
	b, err := m.svc.Book(ctx, "L1", m.slot(t, "tutor-sara", 48*time.Hour))
	if err != nil {
		t.Fatalf("subscriber booking: %v", err)
	}
	if p, _ := m.r.BookingPrices(ctx, []string{b.ID}); len(p) != 0 || m.pay.calls != 0 {
		t.Fatal("a subscription booking carries no price and no payment")
	}
}

// A teacher without a price can't be booked by paying learners.
func TestUnpricedTeacherCannotBeBooked(t *testing.T) {
	m := newMarket(t)
	_, err := m.svc.Book(context.Background(), "L1", m.slot(t, "tutor-dilan", 48*time.Hour))
	if codeOf(err) != "tutor.price_not_set" {
		t.Fatalf("want tutor.price_not_set, got %v", err)
	}
}

// The teacher's own profile shows the price they set.
func TestTeacherSeesOwnPrice(t *testing.T) {
	ctx := context.Background()
	m := newMarket(t)
	_ = m.svc.LinkUser(ctx, "tutor-sara", "sara-user")
	if err := m.svc.SetMyPrice(ctx, "sara-user", 250000); err != nil {
		t.Fatal(err)
	}
	me, err := m.svc.MyTutorProfile(ctx, "sara-user")
	if err != nil || me.SessionPriceToman != 250000 {
		t.Fatalf("own profile price = %+v %v", me, err)
	}
	if err := m.svc.SetMyPrice(ctx, "sara-user", 0); codeOf(err) != "tutor.invalid_price" {
		t.Fatalf("zero price: want tutor.invalid_price, got %v", err)
	}
}

type groupSrc []GroupSession

func (g groupSrc) TutorGroupSessions(context.Context, string) ([]GroupSession, error) { return g, nil }
func (g groupSrc) AllGroupSessions(context.Context) ([]GroupSession, error)           { return g, nil }
func (g groupSrc) StudentGroupSessions(context.Context, string) ([]GroupSession, error) {
	return nil, nil
}

type hourly int

func (h hourly) TutorPayoutPerHourToman(context.Context) int { return int(h) }

// A paid group class pays the teacher their share of the seats, not the
// hourly rate; an unpriced (subscription) class still pays by the hour.
func TestGroupClassEarningsUseSeatShare(t *testing.T) {
	ctx := context.Background()
	m := newMarket(t)
	_ = m.svc.LinkUser(ctx, "tutor-sara", "sara-user")
	m.svc.SetPayoutRate(hourly(60000))
	m.svc.SetGroupSessions(groupSrc{
		{DurationMin: 60, Completed: true, Priced: true, RevenueToman: 300000, TeacherEarnedToman: 210000},
		{DurationMin: 60, Completed: true}, // old subscription class: hourly
	})
	rep, err := m.svc.EarningsReport(ctx, "sara-user")
	if err != nil {
		t.Fatal(err)
	}
	if want := 210000 + 60000; rep.EarningsToman != want || rep.SessionEarningsToman != 210000 {
		t.Fatalf("earnings = %d (session %d), want %d", rep.EarningsToman, rep.SessionEarningsToman, want)
	}
	sum, _ := m.svc.TotalPayout(ctx)
	if sum.PayoutToman != 270000 || sum.PlatformShareToman != 90000 || sum.SessionRevenueToman != 300000 {
		t.Fatalf("payout summary = %+v", sum)
	}
}
