// Package domain holds monetization types: plans, subscriptions, payments, and
// daily free-tier usage. Premium = an active subscription whose period has not
// ended. Free users get a daily quota per action.
package domain

import (
	"context"
	"time"
)

// Subscription products. A subscription is per (user, product): "courses"
// unlocks lessons/film; "classes" unlocks live tutoring + group classes.
const (
	ProductCourses = "courses"
	ProductClasses = "classes"
	// AccessFree marks content open to any signed-in user (no subscription).
	AccessFree = "free"
)

type Plan string

const (
	PlanFree    Plan = "free"
	PlanPremium Plan = "premium"
)

// PlanOption is a purchasable plan in the catalog.
type PlanOption struct {
	ID          string `json:"id"`           // "monthly" | "yearly"
	Title       string `json:"title"`        // message code resolved client-side
	AmountToman int    `json:"amount_toman"` // price shown to the user, in Toman
	Days        int    `json:"days"`         // access granted on success
}

// Subscription is a user's premium window.
type Subscription struct {
	UserID    string
	Product   string // "courses" | "classes"
	Plan      Plan
	Status    string // "active" | "expired"
	PeriodEnd time.Time
	UpdatedAt time.Time
}

func (s *Subscription) Active(now time.Time) bool {
	return s != nil && s.Status == "active" && s.PeriodEnd.After(now)
}

// Payment is a pending/known payment intent, keyed by gateway authority.
type Payment struct {
	Authority   string
	UserID      string
	Product     string // "courses" | "classes"
	PlanID      string
	AmountToman int
	Status      string // "pending" | "paid" | "failed"
	RefID       string
	CreatedAt   time.Time
}

// Entitlements is the user's current access, for display and gating.
type Entitlements struct {
	Plan       Plan       `json:"plan"`
	IsPremium  bool       `json:"is_premium"`
	PeriodEnd  *time.Time `json:"period_end,omitempty"`
	DailyLimit int        `json:"daily_limit"` // 0 = unlimited (premium)
	UsedToday  int        `json:"used_today"`
	Remaining  int        `json:"remaining"` // -1 = unlimited
}

// AccessDecision describes whether a user may open a lesson, and how.
type AccessDecision struct {
	Allowed    bool   `json:"allowed"`
	Reason     string `json:"reason"` // premium | unlocked | free_available | needs_unlock
	IsPremium  bool   `json:"is_premium"`
	FreeUsed   int    `json:"free_used"`
	FreeLimit  int    `json:"free_limit"`
	GemCost    int    `json:"gem_cost"`
	GemBalance int    `json:"gem_balance"`
	CanUseGems bool   `json:"can_use_gems"`
}

type Repository interface {
	GetSubscription(ctx context.Context, userID, product string) (*Subscription, error)
	UpsertSubscription(ctx context.Context, s Subscription) error

	CreatePayment(ctx context.Context, p Payment) error
	GetPayment(ctx context.Context, authority string) (*Payment, error)
	MarkPayment(ctx context.Context, authority, status, refID string) error

	UsageToday(ctx context.Context, userID, action, day string) (int, error)
	IncrUsage(ctx context.Context, userID, action, day string) (int, error)

	// Lesson unlocks: a "gem" unlock is permanent; a "free" unlock counts only for its day.
	LessonUnlocked(ctx context.Context, userID, lessonID, today string) (bool, error)
	FreeUnlocksToday(ctx context.Context, userID, today string) (int, error)
	AddUnlock(ctx context.Context, userID, lessonID, source, day string) error

	// Revenue analytics (admin).
	PaidPaymentTotals(ctx context.Context) (byProduct map[string]int, count int, err error)
	ActiveSubscriptionCounts(ctx context.Context, now time.Time) (map[string]int, error)
	ActiveProductsForUsers(ctx context.Context, userIDs []string, now time.Time) (map[string][]string, error)
	PaymentsForUser(ctx context.Context, userID string) ([]Payment, error)
	SubscriptionsForUser(ctx context.Context, userID string) ([]Subscription, error)
	ActiveSubscriberIDs(ctx context.Context, now time.Time) ([]string, error)
}
