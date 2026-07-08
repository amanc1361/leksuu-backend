package domain

import (
	"context"
	"time"
)

// Repository persists gamification state. Get/DailyGoal return sensible defaults
// (zero state, fresh goal) when nothing is stored yet — never a not-found error.
type Repository interface {
	Get(ctx context.Context, userID string) (*State, error)
	Upsert(ctx context.Context, s *State) error
	DailyGoal(ctx context.Context, userID string, date time.Time, defaultTarget int) (*DailyGoal, error)
	AddDailyXP(ctx context.Context, userID string, date time.Time, defaultTarget, xp int) (*DailyGoal, error)
	// WeeklyTotals sums earned XP per user over [from, to), descending by XP,
	// skipping users with zero XP. Powers the weekly league leaderboard.
	WeeklyTotals(ctx context.Context, from, to time.Time) ([]WeeklyEntry, error)
	// ActiveDays counts distinct goal days for the user in [from, to).
	ActiveDays(ctx context.Context, userID string, from, to time.Time) (int, error)
	// RecapSent / MarkRecapSent implement once-per-week recap dedup.
	RecapSent(ctx context.Context, userID, weekKey string) (bool, error)
	MarkRecapSent(ctx context.Context, userID, weekKey string) error
}

// WeeklyEntry is one user's XP total inside a leaderboard window.
type WeeklyEntry struct {
	UserID string
	XP     int
}
