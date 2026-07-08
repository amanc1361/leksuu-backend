// Package domain holds the platform-wide, admin-editable settings: subscription
// prices and access policies, kept in one record so the owner can change them
// from the admin console without a redeploy.
package domain

import (
	"context"

	"github.com/leksuu/platform/internal/shared/apperr"
)

// Settings is the single configuration record for the whole platform.
type Settings struct {
	FreeDailyLimit int `json:"free_daily_limit"`
	GemUnlockCost  int `json:"gem_unlock_cost"`

	RoomJoinWindowMin int `json:"room_join_window_min"`

	TutorPayoutPerHourToman int `json:"tutor_payout_per_hour_toman"`

	CourseMonthlyToman int `json:"course_monthly_toman"`
	CourseMonthlyDays  int `json:"course_monthly_days"`
	CourseYearlyToman  int `json:"course_yearly_toman"`
	CourseYearlyDays   int `json:"course_yearly_days"`

	ClassMonthlyToman int `json:"class_monthly_toman"`
	ClassMonthlyDays  int `json:"class_monthly_days"`
	ClassYearlyToman  int `json:"class_yearly_toman"`
	ClassYearlyDays   int `json:"class_yearly_days"`
}

// Defaults returns the seed settings (mirrors the previous hardcoded config).
func Defaults() Settings {
	return Settings{
		FreeDailyLimit:          2,
		GemUnlockCost:           50,
		RoomJoinWindowMin:       10,
		TutorPayoutPerHourToman: 80000,
		CourseMonthlyToman:      99000,
		CourseMonthlyDays:       30,
		CourseYearlyToman:       890000,
		CourseYearlyDays:        365,
		ClassMonthlyToman:       149000,
		ClassMonthlyDays:        30,
		ClassYearlyToman:        1290000,
		ClassYearlyDays:         365,
	}
}

// Normalize clamps invalid values back to sane defaults.
func (s *Settings) Normalize() {
	d := Defaults()
	if s.FreeDailyLimit < 0 {
		s.FreeDailyLimit = d.FreeDailyLimit
	}
	if s.GemUnlockCost < 0 {
		s.GemUnlockCost = d.GemUnlockCost
	}
	if s.RoomJoinWindowMin <= 0 {
		s.RoomJoinWindowMin = d.RoomJoinWindowMin
	}
	if s.TutorPayoutPerHourToman < 0 {
		s.TutorPayoutPerHourToman = d.TutorPayoutPerHourToman
	}
	if s.CourseMonthlyDays <= 0 {
		s.CourseMonthlyDays = d.CourseMonthlyDays
	}
	if s.CourseYearlyDays <= 0 {
		s.CourseYearlyDays = d.CourseYearlyDays
	}
	if s.ClassMonthlyDays <= 0 {
		s.ClassMonthlyDays = d.ClassMonthlyDays
	}
	if s.ClassYearlyDays <= 0 {
		s.ClassYearlyDays = d.ClassYearlyDays
	}
	for _, p := range []*int{&s.CourseMonthlyToman, &s.CourseYearlyToman, &s.ClassMonthlyToman, &s.ClassYearlyToman} {
		if *p < 0 {
			*p = 0
		}
	}
}

// Repository persists the single settings record.
type Repository interface {
	Get(ctx context.Context) (*Settings, error)
	Save(ctx context.Context, s *Settings) error
}

// Plan is a purchasable subscription plan for a product ("courses"/"classes").
// (Product, ID) is the identity; ID is the period key billing matches on
// ("monthly", "yearly", or any custom id).
type Plan struct {
	Product     string `json:"product"`
	ID          string `json:"id"`
	Title       string `json:"title"`
	AmountToman int    `json:"amount_toman"`
	Days        int    `json:"days"`
	Position    int    `json:"position"`
	Active      bool   `json:"active"`
}

// PlanRepository stores the admin-managed subscription plans.
type PlanRepository interface {
	List(ctx context.Context) ([]Plan, error)
	ListActiveByProduct(ctx context.Context, product string) ([]Plan, error)
	Upsert(ctx context.Context, p Plan) error
	Delete(ctx context.Context, product, id string) error
}

// Product identifiers for plans.
const (
	ProductCourses = "courses"
	ProductClasses = "classes"
)

var (
	ErrInvalidProduct = apperr.New("settings.invalid_product", apperr.KindValidation)
	ErrInvalidPlan    = apperr.New("settings.invalid_plan", apperr.KindValidation)
)
