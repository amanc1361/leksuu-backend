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
	// Mutate applies one change to a user's state as a single atomic step (a
	// row lock + transaction in Postgres): concurrent awards, spends and claims
	// never lose each other's updates, and the day's goal is updated together
	// with the state. It reports false — writing nothing — when the change's
	// EventKey was already claimed. An error from Apply aborts the change.
	Mutate(ctx context.Context, userID string, ch Change) (bool, error)
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

// Change is one atomic update of a user's gamification state.
type Change struct {
	// Apply edits the locked state; returning an error aborts the change.
	Apply func(*State) error
	// DailyXP is added to Day's goal in the same transaction (0 = none). A new
	// goal row gets the user's own target, or DefaultTarget when unset.
	DailyXP       int
	Day           time.Time
	DefaultTarget int
	// EventKey, when set, makes the change happen at most once per user and
	// key (e.g. "lesson:<id>"); EventXP is recorded alongside it.
	EventKey string
	EventXP  int
}

// WeeklyEntry is one user's XP total inside a leaderboard window.
type WeeklyEntry struct {
	UserID string
	XP     int
}
