package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/leksuu/platform/internal/modules/billing/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
)

type fakeRepo struct {
	sub     *domain.Subscription
	pays    map[string]domain.Payment
	usage   map[string]int
	unlocks map[string][2]string // userID|lessonID -> {source, day}
}

func newFake() *fakeRepo {
	return &fakeRepo{pays: map[string]domain.Payment{}, usage: map[string]int{}, unlocks: map[string][2]string{}}
}

func (r *fakeRepo) GetSubscription(_ context.Context, _ string, _ string) (*domain.Subscription, error) {
	return r.sub, nil
}
func (r *fakeRepo) UpsertSubscription(_ context.Context, s domain.Subscription) error {
	cp := s
	r.sub = &cp
	return nil
}
func (r *fakeRepo) CreatePayment(_ context.Context, p domain.Payment) error {
	r.pays[p.Authority] = p
	return nil
}
func (r *fakeRepo) GetPayment(_ context.Context, a string) (*domain.Payment, error) {
	if p, ok := r.pays[a]; ok {
		return &p, nil
	}
	return nil, nil
}
func (r *fakeRepo) MarkPayment(_ context.Context, a, st, ref string) error {
	if p, ok := r.pays[a]; ok {
		p.Status, p.RefID = st, ref
		r.pays[a] = p
	}
	return nil
}
func (r *fakeRepo) UsageToday(_ context.Context, u, a, d string) (int, error) {
	return r.usage[u+a+d], nil
}
func (r *fakeRepo) IncrUsage(_ context.Context, u, a, d string) (int, error) {
	r.usage[u+a+d]++
	return r.usage[u+a+d], nil
}
func (r *fakeRepo) LessonUnlocked(_ context.Context, u, l, today string) (bool, error) {
	v, ok := r.unlocks[u+"|"+l]
	if !ok {
		return false, nil
	}
	return v[0] == "gem" || (v[0] == "free" && v[1] == today), nil
}
func (r *fakeRepo) FreeUnlocksToday(_ context.Context, u, today string) (int, error) {
	n := 0
	for k, v := range r.unlocks {
		if len(k) > len(u) && k[:len(u)+1] == u+"|" && v[0] == "free" && v[1] == today {
			n++
		}
	}
	return n, nil
}
func (r *fakeRepo) AddUnlock(_ context.Context, u, l, source, day string) error {
	r.unlocks[u+"|"+l] = [2]string{source, day}
	return nil
}

type fakeWallet struct{ bal int }

func (w *fakeWallet) Balance(_ context.Context, _ string) (int, error) { return w.bal, nil }
func (w *fakeWallet) Spend(_ context.Context, _ string, amount int) error {
	if w.bal < amount {
		return apperr.New("billing.insufficient_gems", apperr.KindForbidden)
	}
	w.bal -= amount
	return nil
}

type okProvider struct{}

func (okProvider) Name() string { return "stub" }
func (okProvider) RequestPayment(_ context.Context, _ int, _, cb string) (string, string, error) {
	return "AUTH1", cb + "?Authority=AUTH1&Status=OK", nil
}
func (okProvider) VerifyPayment(_ context.Context, _ string, _ int) (string, error) {
	return "REF1", nil
}

func newSvc(r *fakeRepo) *Service {
	return NewService(r, okProvider{}, &fakeWallet{bal: 1000}, Config{
		Plans:          []domain.PlanOption{{ID: "monthly", AmountToman: 99000, Days: 30}},
		FreeDailyLimit: 3, GemUnlockCost: 50, Action: "lesson",
		CallbackURL: "http://cb", SuccessURL: "http://ok", FailURL: "http://fail",
	})
}

func TestFreeQuotaConsumeAndBlock(t *testing.T) {
	r := newFake()
	s := newSvc(r)
	for i := 1; i <= 3; i++ {
		ent, err := s.Consume(context.Background(), "u1", "lesson")
		if err != nil {
			t.Fatalf("consume %d errored: %v", i, err)
		}
		if ent.Remaining != 3-i {
			t.Fatalf("after consume %d remaining=%d want %d", i, ent.Remaining, 3-i)
		}
	}
	if _, err := s.Consume(context.Background(), "u1", "lesson"); err == nil {
		t.Fatal("expected quota_exceeded on 4th consume")
	} else if ae, ok := err.(*apperr.Error); !ok || ae.Code != "billing.quota_exceeded" {
		t.Fatalf("expected billing.quota_exceeded, got %v", err)
	}
}

func TestSubscribeThenCallbackActivatesPremium(t *testing.T) {
	r := newFake()
	s := newSvc(r)
	if _, err := s.Subscribe(context.Background(), "u1", "courses", "monthly"); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	redirect := s.HandleCallback(context.Background(), "AUTH1", "OK")
	if redirect != "http://ok" {
		t.Fatalf("redirect=%q want success", redirect)
	}
	ent, _ := s.Entitlements(context.Background(), "u1")
	if !ent.IsPremium || ent.Remaining != -1 {
		t.Fatalf("expected premium unlimited, got %+v", ent)
	}
	// premium consume never blocks
	for i := 0; i < 10; i++ {
		if _, err := s.Consume(context.Background(), "u1", "lesson"); err != nil {
			t.Fatalf("premium consume blocked: %v", err)
		}
	}
}

func TestCallbackFailsGracefully(t *testing.T) {
	r := newFake()
	s := newSvc(r)
	_, _ = s.Subscribe(context.Background(), "u1", "courses", "monthly")
	if got := s.HandleCallback(context.Background(), "AUTH1", "NOK"); got != "http://fail" {
		t.Fatalf("expected fail redirect, got %q", got)
	}
}

var _ = time.Now

// Outside the free tier a lesson needs the learning subscription: no daily
// free unlocks and no gem unlocks are offered (and no gems are spent), an
// unlock already made today is still honoured, and subscribing opens it.
func TestLessonAccessNeedsSubscriptionOutsideFreeTier(t *testing.T) {
	ctx := context.Background()
	r := newFake()
	w := &fakeWallet{bal: 120}
	s := NewService(r, okProvider{}, w, Config{FreeDailyLimit: 2, GemUnlockCost: 50, Action: "lesson"})
	if d, _ := s.LessonAccess(ctx, "u", "L1"); d.Allowed || d.Reason != "subscription_required" || d.CanUseGems || d.FreeLimit != 0 {
		t.Fatalf("L1 access = %+v (no free or gem path expected)", d)
	}
	if _, err := s.UnlockLesson(ctx, "u", "L1"); err == nil {
		t.Fatal("unlocking without a subscription must fail")
	}
	if w.bal != 120 {
		t.Fatalf("no gems may be spent, balance %d", w.bal)
	}
	// An unlock made earlier today (before this change) is honoured.
	_ = r.AddUnlock(ctx, "u", "L2", "gem", s.day())
	if d, _ := s.LessonAccess(ctx, "u", "L2"); !d.Allowed || d.Reason != "unlocked" {
		t.Fatalf("existing unlock should be honoured: %+v", d)
	}
	// A subscriber gets in.
	_ = r.UpsertSubscription(ctx, domain.Subscription{UserID: "u", Product: domain.ProductCourses, Status: "active", PeriodEnd: time.Now().Add(24 * time.Hour)})
	if d, _ := s.LessonAccess(ctx, "u", "L1"); !d.Allowed || !d.IsPremium {
		t.Fatalf("subscriber access = %+v", d)
	}
}

func (r *fakeRepo) PaidPaymentTotals(_ context.Context) (map[string]int, int, error) {
	return map[string]int{}, 0, nil
}
func (r *fakeRepo) ActiveSubscriptionCounts(_ context.Context, _ time.Time) (map[string]int, error) {
	return map[string]int{}, nil
}

func (r *fakeRepo) ActiveProductsForUsers(_ context.Context, _ []string, _ time.Time) (map[string][]string, error) {
	return map[string][]string{}, nil
}

func (r *fakeRepo) PaymentsForUser(_ context.Context, _ string) ([]domain.Payment, error) {
	return nil, nil
}
func (r *fakeRepo) SubscriptionsForUser(_ context.Context, _ string) ([]domain.Subscription, error) {
	return nil, nil
}

func (r *fakeRepo) ActiveSubscriberIDs(_ context.Context, _ time.Time) ([]string, error) {
	return nil, nil
}

type noAccess struct{}

func (noAccess) LessonRequirement(context.Context, string) (string, error) {
	return "", apperr.New("lesson.not_found", apperr.KindNotFound)
}

// A lesson that does not exist (or is unpublished) never consumes a free
// unlock or gems.
func TestUnlockMissingLessonSpendsNothing(t *testing.T) {
	r := newFake()
	s := newSvc(r)
	s.SetAccessResolver(noAccess{})
	if _, err := s.UnlockLesson(context.Background(), "u1", "no-such-lesson"); err == nil {
		t.Fatal("unlocking a missing lesson must fail")
	}
	if n, _ := r.FreeUnlocksToday(context.Background(), "u1", s.day()); n != 0 {
		t.Fatalf("free unlocks used = %d, want 0", n)
	}
}

type unavailableProvider struct{ okProvider }

func (unavailableProvider) RequestPayment(context.Context, int, string, string) (string, string, error) {
	return "", "", ErrPaymentsUnavailable
}

func TestSubscribeWithoutGatewayIsUnavailable(t *testing.T) {
	r := newFake()
	s := NewService(r, unavailableProvider{}, &fakeWallet{}, Config{Plans: []domain.PlanOption{{ID: "monthly", AmountToman: 1, Days: 30}}})
	if _, err := s.Subscribe(context.Background(), "u1", "courses", "monthly"); err != ErrPaymentsUnavailable {
		t.Fatalf("want ErrPaymentsUnavailable, got %v", err)
	}
	if len(r.pays) != 0 {
		t.Fatal("no payment may be created")
	}
}
