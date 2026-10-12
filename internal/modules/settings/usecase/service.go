// Package usecase exposes admin settings reads/writes and a pricing view that
// the billing module consumes live (so price changes take effect immediately).
package usecase

import (
	"context"
	"strings"

	"github.com/leksuu/platform/internal/modules/settings/domain"
)

type Service struct {
	repo  domain.Repository
	plans domain.PlanRepository
}

func NewService(repo domain.Repository) *Service { return &Service{repo: repo} }

// SetPlans wires the dynamic subscription-plan repository (optional; when unset,
// Pricing falls back to the fixed config-based plans).
func (s *Service) SetPlans(p domain.PlanRepository) { s.plans = p }

// PlansAll returns every plan (admin management).
func (s *Service) PlansAll(ctx context.Context) ([]domain.Plan, error) {
	if s.plans == nil {
		return nil, nil
	}
	return s.plans.List(ctx)
}

// UpsertPlan creates or updates a plan after validation.
func (s *Service) UpsertPlan(ctx context.Context, p domain.Plan) error {
	if s.plans == nil {
		return nil
	}
	if p.Product != domain.ProductCourses && p.Product != domain.ProductClasses {
		return domain.ErrInvalidProduct
	}
	p.ID = strings.TrimSpace(p.ID)
	p.Title = strings.TrimSpace(p.Title)
	if p.ID == "" || p.Title == "" || p.AmountToman < 0 || p.Days <= 0 {
		return domain.ErrInvalidPlan
	}
	return s.plans.Upsert(ctx, p)
}

// DeletePlan removes a plan by (product, id).
func (s *Service) DeletePlan(ctx context.Context, product, id string) error {
	if s.plans == nil {
		return nil
	}
	return s.plans.Delete(ctx, product, id)
}

// Get returns the current settings (defaults if unset).
func (s *Service) Get(ctx context.Context) (*domain.Settings, error) {
	return s.repo.Get(ctx)
}

// Update validates and persists the settings.
func (s *Service) Update(ctx context.Context, in domain.Settings) (*domain.Settings, error) {
	in.Normalize()
	if err := s.repo.Save(ctx, &in); err != nil {
		return nil, err
	}
	return &in, nil
}

// --- billing.SettingsProvider implementation ---

// PlanOptionView mirrors billing.domain.PlanOption without importing it (avoids
// a dependency cycle); billing maps these into its own type.
type PlanOptionView struct {
	ID          string
	Title       string
	AmountToman int
	Days        int
}

// Pricing is the subset of settings billing needs, resolved live.
type Pricing struct {
	FreeDailyLimit int
	GemUnlockCost  int
	CoursePlans    []PlanOptionView
	ClassPlans     []PlanOptionView
}

// Pricing returns the live course-subscription pricing for billing. Plans come
// from the dynamic plan repository when available; otherwise it falls back to
// the fixed config-based monthly/yearly plans.
func (s *Service) Pricing(ctx context.Context) (Pricing, error) {
	cfg, err := s.repo.Get(ctx)
	if err != nil {
		return Pricing{}, err
	}
	p := Pricing{FreeDailyLimit: cfg.FreeDailyLimit, GemUnlockCost: cfg.GemUnlockCost}
	if s.plans != nil {
		course, _ := s.plans.ListActiveByProduct(ctx, domain.ProductCourses)
		class, _ := s.plans.ListActiveByProduct(ctx, domain.ProductClasses)
		if len(course) > 0 || len(class) > 0 {
			p.CoursePlans = toViews(course)
			p.ClassPlans = toViews(class)
			return p, nil
		}
	}
	// Fallback: the previously hardcoded plans from the settings record.
	p.CoursePlans = []PlanOptionView{
		{ID: "monthly", Title: "ماهانه", AmountToman: cfg.CourseMonthlyToman, Days: cfg.CourseMonthlyDays},
		{ID: "yearly", Title: "سالانه", AmountToman: cfg.CourseYearlyToman, Days: cfg.CourseYearlyDays},
	}
	p.ClassPlans = []PlanOptionView{
		{ID: "monthly", Title: "ماهانه", AmountToman: cfg.ClassMonthlyToman, Days: cfg.ClassMonthlyDays},
		{ID: "yearly", Title: "سالانه", AmountToman: cfg.ClassYearlyToman, Days: cfg.ClassYearlyDays},
	}
	return p, nil
}

func toViews(plans []domain.Plan) []PlanOptionView {
	out := make([]PlanOptionView, 0, len(plans))
	for _, pl := range plans {
		out = append(out, PlanOptionView{ID: pl.ID, Title: pl.Title, AmountToman: pl.AmountToman, Days: pl.Days})
	}
	return out
}

// RoomJoinWindowMin exposes the live class-room join window (for the tutor/classroom modules).
func (s *Service) RoomJoinWindowMin(ctx context.Context) int {
	cfg, err := s.repo.Get(ctx)
	if err != nil {
		return domain.Defaults().RoomJoinWindowMin
	}
	return cfg.RoomJoinWindowMin
}

// TutorPayoutPerHourToman returns the per-hour tutor payout rate (Toman).
func (s *Service) TutorPayoutPerHourToman(ctx context.Context) int {
	cur, err := s.Get(ctx)
	if err != nil {
		return domain.Defaults().TutorPayoutPerHourToman
	}
	return cur.TutorPayoutPerHourToman
}

// TeacherSharePercent returns the teacher's share of a session price (0–100).
func (s *Service) TeacherSharePercent(ctx context.Context) int {
	cur, err := s.Get(ctx)
	if err != nil {
		return domain.Defaults().TeacherSharePercent
	}
	return cur.TeacherSharePercent
}
