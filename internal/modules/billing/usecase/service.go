// Package usecase implements freemium entitlements + subscription purchase via a
// swappable PaymentProvider port (stub for dev, ZarinPal for Iran). Premium
// unlocks unlimited usage; free users get a daily per-action quota.
package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/leksuu/platform/internal/modules/billing/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
)

// PaymentProvider abstracts a payment gateway. RequestPayment starts a payment
// and returns the gateway authority + the URL to send the user to. VerifyPayment
// confirms a returned authority and yields the gateway reference id.
type PaymentProvider interface {
	Name() string
	RequestPayment(ctx context.Context, amountToman int, desc, callbackURL string) (authority, payURL string, err error)
	VerifyPayment(ctx context.Context, authority string, amountToman int) (refID string, err error)
}

// Wallet is the spendable-gem ledger (gamification satisfies it).
type Wallet interface {
	Balance(ctx context.Context, userID string) (int, error)
	Spend(ctx context.Context, userID string, amount int) error
}

// ErrPaymentsUnavailable is returned when no real payment gateway is
// configured, so no purchase can be started or verified.
var ErrPaymentsUnavailable = apperr.New("billing.payments_unavailable", apperr.KindConflict)

type Config struct {
	Plans          []domain.PlanOption
	FreeDailyLimit int
	GemUnlockCost  int    // gems to unlock one extra lesson
	Action         string // the default gated action, e.g. "lesson"
	CallbackURL    string // gateway redirects here (our /billing/callback)
	SuccessURL     string // frontend page after success
	FailURL        string // frontend page after failure
}

type Service struct {
	// one-off purchases settled by other modules, by product
	fulfillers map[string]SessionFulfiller
	repo       domain.Repository
	provider   PaymentProvider
	wallet     Wallet
	cfg        Config
	settings   SettingsProvider
	access     AccessResolver
	now        func() time.Time
}

// Pricing is the live course-subscription pricing billing reads from settings.
type Pricing struct {
	FreeDailyLimit int
	GemUnlockCost  int
	CoursePlans    []domain.PlanOption
	ClassPlans     []domain.PlanOption
}

// SettingsProvider supplies admin-editable pricing live (optional; nil-safe).
type SettingsProvider interface {
	Pricing(ctx context.Context) (Pricing, error)
}

// SetSettings wires the live pricing source (admin settings).
func (s *Service) SetSettings(p SettingsProvider) { s.settings = p }

// AccessResolver returns a lesson's effective access requirement
// ("free" | "courses" | "classes"), or an error (e.g. not found) for a lesson a
// learner cannot reach. Optional; nil-safe.
type AccessResolver interface {
	LessonRequirement(ctx context.Context, lessonID string) (string, error)
}

// SetAccessResolver wires the per-lesson access requirement source.
func (s *Service) SetAccessResolver(a AccessResolver) { s.access = a }

// lessonRequirement resolves a lesson's effective requirement (default
// courses). A lesson that does not exist (or is unpublished) is an error, so no
// quota or gems are ever spent on it.
func (s *Service) lessonRequirement(ctx context.Context, lessonID string) (string, error) {
	if s.access == nil {
		return domain.ProductCourses, nil
	}
	r, err := s.access.LessonRequirement(ctx, lessonID)
	if err != nil {
		return "", err
	}
	if r == "" {
		return domain.ProductCourses, nil
	}
	return r, nil
}

// pricing resolves the current pricing from settings, falling back to the
// static config when settings are unavailable.
func (s *Service) pricing(ctx context.Context) Pricing {
	if s.settings != nil {
		if pr, err := s.settings.Pricing(ctx); err == nil && len(pr.CoursePlans) > 0 {
			return pr
		}
	}
	return Pricing{FreeDailyLimit: s.cfg.FreeDailyLimit, GemUnlockCost: s.cfg.GemUnlockCost, CoursePlans: s.cfg.Plans}
}

// plansFor returns the purchasable plans for a product (course or class).
func (s *Service) plansFor(ctx context.Context, product string) []domain.PlanOption {
	pr := s.pricing(ctx)
	if product == domain.ProductClasses {
		return pr.ClassPlans
	}
	return pr.CoursePlans
}

// planFor resolves a single plan by product + period ("monthly"/"yearly").
func (s *Service) planFor(ctx context.Context, product, period string) (domain.PlanOption, bool) {
	for _, p := range s.plansFor(ctx, product) {
		if p.ID == period {
			return p, true
		}
	}
	return domain.PlanOption{}, false
}

func NewService(repo domain.Repository, provider PaymentProvider, wallet Wallet, cfg Config) *Service {
	if cfg.Action == "" {
		cfg.Action = "lesson"
	}
	if cfg.GemUnlockCost <= 0 {
		cfg.GemUnlockCost = 50
	}
	return &Service{repo: repo, provider: provider, wallet: wallet, cfg: cfg, now: time.Now}
}

func (s *Service) gemBalance(ctx context.Context, userID string) int {
	if s.wallet == nil {
		return 0
	}
	b, err := s.wallet.Balance(ctx, userID)
	if err != nil {
		return 0
	}
	return b
}

// LessonAccess reports whether the user may open a lesson, and how (read-only).
func (s *Service) LessonAccess(ctx context.Context, userID, lessonID string) (domain.AccessDecision, error) {
	if lessonID == "" {
		return domain.AccessDecision{}, apperr.ErrBadRequest
	}
	req, err := s.lessonRequirement(ctx, lessonID)
	if err != nil {
		return domain.AccessDecision{}, err
	}
	switch req {
	case domain.AccessFree:
		return domain.AccessDecision{Allowed: true, Reason: "free"}, nil
	case domain.ProductClasses:
		ok, err := s.HasProduct(ctx, userID, domain.ProductClasses)
		if err != nil {
			return domain.AccessDecision{}, err
		}
		if ok {
			return domain.AccessDecision{Allowed: true, Reason: "premium", IsPremium: true}, nil
		}
		return domain.AccessDecision{Allowed: false, Reason: "classes_required"}, nil
	}
	sub, err := s.repo.GetSubscription(ctx, userID, domain.ProductCourses)
	if err != nil {
		return domain.AccessDecision{}, err
	}
	if sub.Active(s.now()) {
		return domain.AccessDecision{Allowed: true, Reason: "premium", IsPremium: true}, nil
	}
	// Outside the free tier (each course's first chapter) a lesson needs the
	// learning subscription. Daily free unlocks and gem unlocks are no longer
	// offered; an unlock already made today is still honoured.
	unlocked, err := s.repo.LessonUnlocked(ctx, userID, lessonID, s.day())
	if err != nil {
		return domain.AccessDecision{}, err
	}
	if unlocked {
		return domain.AccessDecision{Allowed: true, Reason: "unlocked"}, nil
	}
	return domain.AccessDecision{Allowed: false, Reason: "subscription_required"}, nil
}

// UnlockLesson grants access: free slot if available, else spends gems. Premium
// users always pass. Returns the resulting access decision.
func (s *Service) UnlockLesson(ctx context.Context, userID, lessonID string) (domain.AccessDecision, error) {
	// Beyond the free tier (each course's first chapter) a lesson is opened by
	// the subscription only: no daily free unlocks, no gem unlocks.
	d, err := s.LessonAccess(ctx, userID, lessonID)
	if err != nil || d.Allowed {
		return d, err
	}
	if d.Reason == "classes_required" {
		return domain.AccessDecision{}, apperr.New("billing.class_subscription_required", apperr.KindForbidden)
	}
	return domain.AccessDecision{}, apperr.New("billing.premium_required", apperr.KindForbidden)
}

func (s *Service) ProviderName() string { return s.provider.Name() }

// LessonAllowed reports whether the user may open the lesson right now
// (subscription, class plan, free lesson, or today's unlock) — read-only. The
// course module uses it to withhold locked content and to refuse grading or
// completing it.
func (s *Service) LessonAllowed(ctx context.Context, userID, lessonID string) (bool, error) {
	d, err := s.LessonAccess(ctx, userID, lessonID)
	if err != nil {
		return false, err
	}
	return d.Allowed, nil
}
func (s *Service) Plans(ctx context.Context, product string) []domain.PlanOption {
	return s.plansFor(ctx, product)
}
func (s *Service) day() string { return s.now().UTC().Format("2006-01-02") }

// Entitlements returns the user's current access (for display + UI gating).
func (s *Service) Entitlements(ctx context.Context, userID string) (domain.Entitlements, error) {
	sub, err := s.repo.GetSubscription(ctx, userID, domain.ProductCourses)
	if err != nil {
		return domain.Entitlements{}, err
	}
	if sub.Active(s.now()) {
		pe := sub.PeriodEnd
		return domain.Entitlements{Plan: domain.PlanPremium, IsPremium: true, PeriodEnd: &pe, DailyLimit: 0, Remaining: -1}, nil
	}
	used, err := s.repo.UsageToday(ctx, userID, s.cfg.Action, s.day())
	if err != nil {
		return domain.Entitlements{}, err
	}
	limit := s.pricing(ctx).FreeDailyLimit
	rem := limit - used
	if rem < 0 {
		rem = 0
	}
	return domain.Entitlements{Plan: domain.PlanFree, IsPremium: false, DailyLimit: limit, UsedToday: used, Remaining: rem}, nil
}

// Consume checks the daily quota for free users and increments it. Premium users
// are unlimited (no increment). Returns the updated entitlements, or a
// quota_exceeded error for free users who are out of allowance.
func (s *Service) Consume(ctx context.Context, userID, action string) (domain.Entitlements, error) {
	if action == "" {
		action = s.cfg.Action
	}
	sub, err := s.repo.GetSubscription(ctx, userID, domain.ProductCourses)
	if err != nil {
		return domain.Entitlements{}, err
	}
	if sub.Active(s.now()) {
		pe := sub.PeriodEnd
		return domain.Entitlements{Plan: domain.PlanPremium, IsPremium: true, PeriodEnd: &pe, DailyLimit: 0, Remaining: -1}, nil
	}
	limit := s.pricing(ctx).FreeDailyLimit
	used, err := s.repo.UsageToday(ctx, userID, action, s.day())
	if err != nil {
		return domain.Entitlements{}, err
	}
	if used >= limit {
		return domain.Entitlements{}, apperr.New("billing.quota_exceeded", apperr.KindForbidden)
	}
	newUsed, err := s.repo.IncrUsage(ctx, userID, action, s.day())
	if err != nil {
		return domain.Entitlements{}, err
	}
	rem := limit - newUsed
	if rem < 0 {
		rem = 0
	}
	return domain.Entitlements{Plan: domain.PlanFree, DailyLimit: limit, UsedToday: newUsed, Remaining: rem}, nil
}

// Subscribe starts a payment for the given plan and returns the gateway pay URL.
func (s *Service) Subscribe(ctx context.Context, userID, product, period string) (string, error) {
	if product == "" {
		product = domain.ProductCourses
	}
	// Online classes are paid per session now; existing class subscriptions
	// keep working until they end, but no new ones are sold.
	if product == domain.ProductClasses {
		return "", apperr.New("billing.classes_per_session", apperr.KindValidation)
	}
	plan, ok := s.planFor(ctx, product, period)
	if !ok {
		return "", apperr.ErrBadRequest
	}
	desc := fmt.Sprintf("Leksuu %s %s subscription", product, plan.ID)
	authority, payURL, err := s.provider.RequestPayment(ctx, plan.AmountToman, desc, s.cfg.CallbackURL)
	if errors.Is(err, ErrPaymentsUnavailable) {
		return "", ErrPaymentsUnavailable
	}
	if err != nil || authority == "" {
		return "", apperr.New("billing.payment_failed", apperr.KindInternal)
	}
	if err := s.repo.CreatePayment(ctx, domain.Payment{
		Authority: authority, UserID: userID, Product: product, PlanID: plan.ID,
		AmountToman: plan.AmountToman, Status: "pending", CreatedAt: s.now(),
	}); err != nil {
		return "", err
	}
	return payURL, nil
}

// One-off products settled by other modules: a 1:1 session (tutor) and a
// seat in a group class (classroom).
const (
	ProductSession    = "session"
	ProductGroupClass = "group_class"
)

// SessionFulfiller settles a session checkout once its payment resolves.
type SessionFulfiller interface {
	FulfillCheckout(ctx context.Context, checkoutID string, paid bool) error
}

// SetSessionFulfiller wires the tutor module (1:1 sessions).
func (s *Service) SetSessionFulfiller(f SessionFulfiller) { s.SetFulfiller(ProductSession, f) }

// SetFulfiller wires the module that settles payments for a one-off product.
func (s *Service) SetFulfiller(product string, f SessionFulfiller) {
	if s.fulfillers == nil {
		s.fulfillers = map[string]SessionFulfiller{}
	}
	s.fulfillers[product] = f
}

// StartSessionPayment opens a gateway payment for a 1:1 session checkout.
func (s *Service) StartSessionPayment(ctx context.Context, userID, checkoutID string, amountToman int, desc string) (string, error) {
	return s.startOneOff(ctx, userID, ProductSession, checkoutID, amountToman, desc)
}

// StartClassPayment opens a gateway payment for a group-class seat checkout.
func (s *Service) StartClassPayment(ctx context.Context, userID, checkoutID string, amountToman int, desc string) (string, error) {
	return s.startOneOff(ctx, userID, ProductGroupClass, checkoutID, amountToman, desc)
}

// startOneOff opens a gateway payment; the payment row keeps the checkout id
// so the callback can hand the result to the product's module.
func (s *Service) startOneOff(ctx context.Context, userID, product, checkoutID string, amountToman int, desc string) (string, error) {
	authority, payURL, err := s.provider.RequestPayment(ctx, amountToman, desc, s.cfg.CallbackURL)
	if err != nil {
		return "", err
	}
	if err := s.repo.CreatePayment(ctx, domain.Payment{
		Authority: authority, UserID: userID, Product: product, PlanID: checkoutID,
		AmountToman: amountToman, Status: "pending", CreatedAt: s.now(),
	}); err != nil {
		return "", err
	}
	return payURL, nil
}

// HandleCallback verifies a returned payment and activates the subscription.
// Returns the frontend URL to redirect the user to.
func (s *Service) HandleCallback(ctx context.Context, authority, status string) string {
	pay, err := s.repo.GetPayment(ctx, authority)
	if err != nil || pay == nil {
		return s.cfg.FailURL
	}
	if f, ok := s.fulfillers[pay.Product]; ok || pay.Product == ProductSession || pay.Product == ProductGroupClass {
		return s.handleOneOffCallback(ctx, pay, status, f)
	}
	// A payment is settled once: a repeated callback (refresh, back button, a
	// replayed URL) reports the outcome but never grants anything again, and
	// a "failed" callback can't overwrite a payment that was already paid.
	switch pay.Status {
	case "paid":
		return s.cfg.SuccessURL
	case "failed":
		return s.cfg.FailURL
	}
	if !strings.EqualFold(status, "OK") {
		_, _ = s.repo.TransitionPayment(ctx, authority, "pending", "failed", "")
		return s.cfg.FailURL
	}
	refID, err := s.provider.VerifyPayment(ctx, authority, pay.AmountToman)
	if err != nil {
		_, _ = s.repo.TransitionPayment(ctx, authority, "pending", "failed", "")
		return s.cfg.FailURL
	}
	won, err := s.repo.TransitionPayment(ctx, authority, "pending", "paid", refID)
	if err != nil {
		return s.cfg.FailURL
	}
	if !won { // a concurrent callback settled it
		return s.cfg.SuccessURL
	}
	product := pay.Product
	if product == "" {
		product = domain.ProductCourses
	}
	plan, ok := s.planFor(ctx, product, pay.PlanID)
	if !ok {
		return s.cfg.FailURL
	}
	// A renewal adds the plan to what is left, so no paid day is lost.
	end := s.now().AddDate(0, 0, plan.Days)
	if cur, _ := s.repo.GetSubscription(ctx, pay.UserID, product); cur != nil && cur.Active(s.now()) {
		end = cur.PeriodEnd.AddDate(0, 0, plan.Days)
	}
	_ = s.repo.UpsertSubscription(ctx, domain.Subscription{
		UserID: pay.UserID, Product: product, Plan: domain.PlanPremium, Status: "active", PeriodEnd: end, UpdatedAt: s.now(),
	})
	return s.cfg.SuccessURL
}

// RequirePremium returns billing.premium_required unless the user has an active
// subscription. Used by premium-only features (pronunciation, podcast, film, AI).
func (s *Service) RequirePremium(ctx context.Context, userID string) error {
	return s.RequireProduct(ctx, userID, domain.ProductCourses, "billing.premium_required")
}

// HasProduct reports whether the user has an active subscription for a product.
func (s *Service) HasProduct(ctx context.Context, userID, product string) (bool, error) {
	sub, err := s.repo.GetSubscription(ctx, userID, product)
	if err != nil {
		return false, err
	}
	return sub.Active(s.now()), nil
}

// RequireProduct errors with the given code unless the product is active.
func (s *Service) RequireProduct(ctx context.Context, userID, product, code string) error {
	ok, err := s.HasProduct(ctx, userID, product)
	if err != nil {
		return err
	}
	if !ok {
		return apperr.New(code, apperr.KindForbidden)
	}
	return nil
}

// RequireClass gates live-class access behind an active class subscription.
func (s *Service) RequireClass(ctx context.Context, userID string) error {
	return s.RequireProduct(ctx, userID, domain.ProductClasses, "billing.class_subscription_required")
}

// ProductEntitlement is a product subscription's status, for display.
type ProductEntitlement struct {
	Product   string     `json:"product"`
	Active    bool       `json:"active"`
	PeriodEnd *time.Time `json:"period_end,omitempty"`
}

// Entitlement returns a product subscription's current state.
func (s *Service) Entitlement(ctx context.Context, userID, product string) (ProductEntitlement, error) {
	sub, err := s.repo.GetSubscription(ctx, userID, product)
	if err != nil {
		return ProductEntitlement{}, err
	}
	out := ProductEntitlement{Product: product, Active: sub.Active(s.now())}
	if out.Active {
		pe := sub.PeriodEnd
		out.PeriodEnd = &pe
	}
	return out, nil
}

// RevenueSummary is the platform's subscription income picture, for the admin.
type RevenueSummary struct {
	TotalToman      int            `json:"total_toman"`
	ByProduct       map[string]int `json:"by_product"` // courses/classes -> toman
	PaidPayments    int            `json:"paid_payments"`
	ActiveByProduct map[string]int `json:"active_by_product"` // courses/classes -> active subs
	ActiveTotal     int            `json:"active_total"`
}

// RevenueSummary aggregates paid payments and active subscriptions by product.
func (s *Service) RevenueSummary(ctx context.Context) (*RevenueSummary, error) {
	by, count, err := s.repo.PaidPaymentTotals(ctx)
	if err != nil {
		return nil, err
	}
	active, err := s.repo.ActiveSubscriptionCounts(ctx, s.now())
	if err != nil {
		return nil, err
	}
	total := 0
	for _, v := range by {
		total += v
	}
	activeTotal := 0
	for _, v := range active {
		activeTotal += v
	}
	return &RevenueSummary{
		TotalToman: total, ByProduct: by, PaidPayments: count,
		ActiveByProduct: active, ActiveTotal: activeTotal,
	}, nil
}

// UserSubscriptionStatuses returns the active products per user (for admin lists).
func (s *Service) UserSubscriptionStatuses(ctx context.Context, userIDs []string) (map[string][]string, error) {
	return s.repo.ActiveProductsForUsers(ctx, userIDs, s.now())
}

// ActiveSubscriberIDs returns all user IDs with any active subscription.
func (s *Service) ActiveSubscriberIDs(ctx context.Context) ([]string, error) {
	return s.repo.ActiveSubscriberIDs(ctx, s.now())
}

// GrantSubscription comps a user an active subscription for `days` (admin). If a
// subscription is already active, the new period extends it.
func (s *Service) GrantSubscription(ctx context.Context, userID, product string, days int) error {
	if product != domain.ProductCourses && product != domain.ProductClasses {
		return apperr.ErrBadRequest
	}
	if days <= 0 {
		days = 30
	}
	end := s.now().AddDate(0, 0, days)
	if cur, _ := s.repo.GetSubscription(ctx, userID, product); cur.Active(s.now()) && cur.PeriodEnd.After(s.now()) {
		end = cur.PeriodEnd.AddDate(0, 0, days)
	}
	return s.repo.UpsertSubscription(ctx, domain.Subscription{
		UserID: userID, Product: product, Plan: domain.PlanPremium, Status: "active", PeriodEnd: end, UpdatedAt: s.now(),
	})
}

// SubscriptionDetail / PaymentView / UserBillingDetail power the admin per-user
// financial view.
type SubscriptionDetail struct {
	Product   string    `json:"product"`
	Active    bool      `json:"active"`
	PeriodEnd time.Time `json:"period_end"`
}

type PaymentView struct {
	Product     string    `json:"product"`
	AmountToman int       `json:"amount_toman"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

type UserBillingDetail struct {
	Subscriptions  []SubscriptionDetail `json:"subscriptions"`
	Payments       []PaymentView        `json:"payments"`
	TotalPaidToman int                  `json:"total_paid_toman"`
}

// UserBillingDetail returns a user's subscriptions and payment history (admin).
func (s *Service) UserBillingDetail(ctx context.Context, userID string) (*UserBillingDetail, error) {
	now := s.now()
	subs, err := s.repo.SubscriptionsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	pays, err := s.repo.PaymentsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := &UserBillingDetail{Subscriptions: []SubscriptionDetail{}, Payments: []PaymentView{}}
	for _, sub := range subs {
		out.Subscriptions = append(out.Subscriptions, SubscriptionDetail{
			Product: sub.Product, Active: sub.Active(now), PeriodEnd: sub.PeriodEnd,
		})
	}
	for _, p := range pays {
		out.Payments = append(out.Payments, PaymentView{
			Product: p.Product, AmountToman: p.AmountToman, Status: p.Status, CreatedAt: p.CreatedAt,
		})
		if p.Status == "paid" {
			out.TotalPaidToman += p.AmountToman
		}
	}
	return out, nil
}

// handleOneOffCallback verifies a one-off payment and hands the result to the
// product's module, which books it (or releases what it held).
func (s *Service) handleOneOffCallback(ctx context.Context, pay *domain.Payment, status string, f SessionFulfiller) string {
	kind := "kind=session"
	if pay.Product == ProductGroupClass {
		kind = "kind=class"
	}
	switch pay.Status { // already settled: report, don't settle again
	case "paid":
		return withQuery(s.cfg.SuccessURL, kind)
	case "failed":
		return withQuery(s.cfg.FailURL, kind)
	}
	paid := false
	if strings.EqualFold(status, "OK") {
		if refID, err := s.provider.VerifyPayment(ctx, pay.Authority, pay.AmountToman); err == nil {
			won, err := s.repo.TransitionPayment(ctx, pay.Authority, "pending", "paid", refID)
			if err != nil {
				return withQuery(s.cfg.FailURL, kind)
			}
			if !won { // a concurrent callback settled it
				return withQuery(s.cfg.SuccessURL, kind)
			}
			paid = true
		}
	}
	if !paid {
		if won, _ := s.repo.TransitionPayment(ctx, pay.Authority, "pending", "failed", ""); !won {
			return withQuery(s.cfg.FailURL, kind)
		}
	}
	if f != nil {
		if err := f.FulfillCheckout(ctx, pay.PlanID, paid); err != nil {
			return withQuery(s.cfg.FailURL, kind)
		}
	}
	if paid {
		return withQuery(s.cfg.SuccessURL, kind)
	}
	return withQuery(s.cfg.FailURL, kind)
}

// withQuery appends a query parameter to a URL that may already have some.
func withQuery(u, kv string) string {
	if strings.Contains(u, "?") {
		return u + "&" + kv
	}
	return u + "?" + kv
}
