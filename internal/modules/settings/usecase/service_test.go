package usecase

import (
	"context"
	"testing"

	"github.com/leksuu/platform/internal/modules/settings/domain"
	"github.com/leksuu/platform/internal/modules/settings/repo"
)

func TestSettingsUpdateNormalizeAndPricing(t *testing.T) {
	svc := NewService(repo.NewMemorySettingsRepo())
	ctx := context.Background()

	// Defaults are returned before any save.
	d, _ := svc.Get(ctx)
	if d.FreeDailyLimit != 2 || d.CourseMonthlyToman != 99000 {
		t.Fatalf("unexpected defaults: %+v", d)
	}

	// Update persists and Pricing reflects it live.
	in := domain.Defaults()
	in.CourseMonthlyToman = 120000
	in.FreeDailyLimit = 5
	if _, err := svc.Update(ctx, in); err != nil {
		t.Fatal(err)
	}
	pr, _ := svc.Pricing(ctx)
	if pr.FreeDailyLimit != 5 {
		t.Fatalf("pricing free limit = %d, want 5", pr.FreeDailyLimit)
	}
	var monthly int
	for _, p := range pr.CoursePlans {
		if p.ID == "monthly" {
			monthly = p.AmountToman
		}
	}
	if monthly != 120000 {
		t.Fatalf("monthly plan = %d, want 120000", monthly)
	}

	// Bad values are clamped on save.
	bad := domain.Defaults()
	bad.RoomJoinWindowMin = -3
	bad.CourseMonthlyDays = 0
	out, _ := svc.Update(ctx, bad)
	if out.RoomJoinWindowMin != 10 || out.CourseMonthlyDays != 30 {
		t.Fatalf("normalize failed: %+v", out)
	}
}
