// Package usecase implements freemium entitlements + subscription purchase via a
// swappable PaymentProvider port (stub for dev, ZarinPal for Iran). Premium
// unlocks unlimited usage; free users get a daily per-action quota.
package usecase

import (
	"context"
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
	repo     domain.Repository
	provider PaymentProvider
	wallet   Wallet
	cfg      Config
	settings SettingsProvider
	access   AccessResolver
	now      func() time.Time
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
// ("free" | "courses" | "classes"). Optional; nil-safe.
type AccessResolver interface {
	LessonRequirement(ctx context.Context, lessonID string) (string, error)
}

// SetAccessResolver wires the per-lesson access requirement source.
func (s *Service) SetAccessResolver(a AccessResolver) { s.access = a }

// lessonRequirement resolves a lesson's effective requirement (default courses).
func (s *Service) lessonRequirement(ctx context.Context, lessonID string) string {
	if s.access != nil {
		if r, err := s.access.LessonRequirement(ctx, lessonID); err == nil && r != "" {
			return r
		}
	}
	return domain.ProductCourses
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
	switch s.lessonRequirement(ctx, lessonID) {
	case domain.AccessFree:
		return domain.AccessDecision{Allowed: true, Reason: "free"}, nil
	case domain.ProductClasses:
		if ok, _ := s.HasProduct(ctx, userID, domain.ProductClasses); ok {
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
	today := s.day()
	unlocked, err := s.repo.LessonUnlocked(ctx, userID, lessonID, today)
	if err != nil {
		return domain.AccessDecision{}, err
	}
	freeUsed, err := s.repo.FreeUnlocksToday(ctx, userID, today)
	if err != nil {
		return domain.AccessDecision{}, err
	}
	bal := s.gemBalance(ctx, userID)
	pr := s.pricing(ctx)
	d := domain.AccessDecision{
		FreeUsed: freeUsed, FreeLimit: pr.FreeDailyLimit,
		GemCost: pr.GemUnlockCost, GemBalance: bal, CanUseGems: bal >= pr.GemUnlockCost,
	}
	switch {
	case unlocked:
		d.Allowed, d.Reason = true, "unlocked"
	case freeUsed < pr.FreeDailyLimit:
		d.Reason = "free_available"
	default:
		d.Reason = "needs_unlock"
	}
	return d, nil
}

// UnlockLesson grants access: free slot if available, else spends gems. Premium
// users always pass. Returns the resulting access decision.
func (s *Service) UnlockLesson(ctx context.Context, userID, lessonID string) (domain.AccessDecision, error) {
	if lessonID == "" {
		return domain.AccessDecision{}, apperr.ErrBadRequest
	}
	switch s.lessonRequirement(ctx, lessonID) {
	case domain.AccessFree:
		return domain.AccessDecision{Allowed: true, Reason: "free"}, nil
	case domain.ProductClasses:
		if ok, _ := s.HasProduct(ctx, userID, domain.ProductClasses); ok {
			return domain.AccessDecision{Allowed: true, Reason: "unlocked", IsPremium: true}, nil
		}
		return domain.AccessDecision{}, apperr.New("billing.class_subscription_required", apperr.KindForbidden)
	}
	sub, err := s.repo.GetSubscription(ctx, userID, domain.ProductCourses)
	if err != nil {
		return domain.AccessDecision{}, err
	}
	if sub.Active(s.now()) {
		return domain.AccessDecision{Allowed: true, Reason: "premium", IsPremium: true}, nil
	}
	today := s.day()
	if ok, err := s.repo.LessonUnlocked(ctx, userID, lessonID, today); err != nil {
		return domain.AccessDecision{}, err
	} else if ok {
		return domain.AccessDecision{Allowed: true, Reason: "unlocked"}, nil
	}
	freeUsed, err := s.repo.FreeUnlocksToday(ctx, userID, today)
	if err != nil {
		return domain.AccessDecision{}, err
	}
	pr := s.pricing(ctx)
	if freeUsed < pr.FreeDailyLimit {
		if err := s.repo.AddUnlock(ctx, userID, lessonID, "free", today); err != nil {
			return domain.AccessDecision{}, err
		}
		return domain.AccessDecision{Allowed: true, Reason: "unlocked", FreeUsed: freeUsed + 1, FreeLimit: pr.FreeDailyLimit}, nil
	}
	// out of free slots -> try gems
	bal := s.gemBalance(ctx, userID)
	if s.wallet == nil || bal < pr.GemUnlockCost {
		return domain.AccessDecision{}, apperr.New("billing.needs_unlock", apperr.KindForbidden)
	}
	if err := s.wallet.Spend(ctx, userID, pr.GemUnlockCost); err != nil {
		return domain.AccessDecision{}, err
	}
	if err := s.repo.AddUnlock(ctx, userID, lessonID, "gem", today); err != nil {
		return domain.AccessDecision{}, err
	}
	return domain.AccessDecision{Allowed: true, Reason: "unlocked", GemBalance: bal - pr.GemUnlockCost}, nil
}

func (s *Service) ProviderName() string { return s.provider.Name() }
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
	plan, ok := s.planFor(ctx, product, period)
	if !ok {
		return "", apperr.ErrBadRequest
	}
	desc := fmt.Sprintf("Leksuu %s %s subscription", product, plan.ID)
	authority, payURL, err := s.provider.RequestPayment(ctx, plan.AmountToman, desc, s.cfg.CallbackURL)
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

// HandleCallback verifies a returned payment and activates the subscription.
// Returns the frontend URL to redirect the user to.
func (s *Service) HandleCallback(ctx context.Context, authority, status string) string {
	pay, err := s.repo.GetPayment(ctx, authority)
	if err != nil || pay == nil {
		return s.cfg.FailURL
	}
	if !strings.EqualFold(status, "OK") {
		_ = s.repo.MarkPayment(ctx, authority, "failed", "")
		return s.cfg.FailURL
	}
	refID, err := s.provider.VerifyPayment(ctx, authority, pay.AmountToman)
	if err != nil {
		_ = s.repo.MarkPayment(ctx, authority, "failed", "")
		return s.cfg.FailURL
	}
	_ = s.repo.MarkPayment(ctx, authority, "paid", refID)
	product := pay.Product
	if product == "" {
		product = domain.ProductCourses
	}
	plan, ok := s.planFor(ctx, product, pay.PlanID)
	if !ok {
		return s.cfg.FailURL
	}
	end := s.now().AddDate(0, 0, plan.Days)
	// extend if already active
	if cur, _ := s.repo.GetSubscription(ctx, pay.UserID, product); cur.Active(s.now()) && cur.PeriodEnd.After(end) {
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
