// Package domain holds the gamification bounded context: a user's XP, streak,
// league, and daily goal — the motivation loop. Depends only on shared kernel.
package domain

import "time"

// Leagues, ordered by XP threshold (Duolingo-style tiers).
const (
	LeagueBronze   = "bronze"
	LeagueSilver   = "silver"
	LeagueGold     = "gold"
	LeagueSapphire = "sapphire"
	LeagueRuby     = "ruby"
	LeagueDiamond  = "diamond"
)

// State is a user's gamification snapshot.
type State struct {
	UserID        string
	XP            int
	Gems          int
	Streak        int
	LongestStreak int
	League        string
	LastActive    *time.Time // date of last XP-earning activity (UTC, midnight)
	DailyTargetXP int        // custom per-user daily goal (0 = use default)
	LastReward    *time.Time // date the daily reward was last claimed (UTC, midnight)
	LastMix       *time.Time // date the guided daily mix was last completed (UTC, midnight)
}

// DailyGoal tracks XP earned toward a per-day target.
type DailyGoal struct {
	Date     time.Time
	TargetXP int
	EarnedXP int
	Met      bool
}
