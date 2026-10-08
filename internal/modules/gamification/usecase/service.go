// Package usecase holds the gamification rules: awarding XP, advancing streaks,
// computing leagues, and tracking the daily goal. Depends only on the domain.
package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/leksuu/platform/internal/shared/apperr"

	"github.com/leksuu/platform/internal/modules/gamification/domain"
	"strconv"
)

// DefaultDailyTarget is the XP a learner aims for each day.
const DefaultDailyTarget = 50

// MinDailyTarget / MaxDailyTarget bound a user-chosen daily goal.
const (
	MinDailyTarget = 10
	MaxDailyTarget = 300
)

func targetFor(st *domain.State) int {
	if st.DailyTargetXP > 0 {
		return st.DailyTargetXP
	}
	return DefaultDailyTarget
}

// leagueThresholds maps a minimum XP to a league (checked high to low).
var leagueThresholds = []struct {
	min    int
	league string
}{
	{8000, domain.LeagueDiamond},
	{5000, domain.LeagueRuby},
	{3000, domain.LeagueSapphire},
	{1500, domain.LeagueGold},
	{500, domain.LeagueSilver},
	{0, domain.LeagueBronze},
}

func leagueFor(xp int) string {
	for _, t := range leagueThresholds {
		if xp >= t.min {
			return t.league
		}
	}
	return domain.LeagueBronze
}

type Service struct {
	repo    domain.Repository
	clock   func() time.Time
	names   NameResolver // optional display-name resolver (auth)
	notif   Notifier     // optional; powers the weekly recap
	reviews ReviewQueue  // optional; the daily mix's review step (srs)
}

// ReviewQueue reports how many spaced-repetition cards a learner has due now.
// The srs module implements it; the daily-mix bonus requires an empty queue.
type ReviewQueue interface {
	DueCount(ctx context.Context, userID string) (int, error)
}

// SetReviewQueue wires the late-bound review-queue port.
func (s *Service) SetReviewQueue(q ReviewQueue) { s.reviews = q }

func NewService(repo domain.Repository, clock func() time.Time) *Service {
	return &Service{repo: repo, clock: clock}
}

// StateView is the full snapshot returned to the delivery layer.
type StateView struct {
	State domain.State
	Today domain.DailyGoal
}

// ActivityView is a compact learning-activity snapshot for the admin per-user view.
type ActivityView struct {
	XP            int        `json:"xp"`
	Gems          int        `json:"gems"`
	Streak        int        `json:"streak"`
	LongestStreak int        `json:"longest_streak"`
	League        string     `json:"league"`
	LastActive    *time.Time `json:"last_active"`
}

// UserActivity returns a user's learning-activity snapshot (admin).
func (s *Service) UserActivity(ctx context.Context, userID string) (*ActivityView, error) {
	st, err := s.repo.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &ActivityView{
		XP: st.XP, Gems: st.Gems, Streak: st.Streak, LongestStreak: st.LongestStreak,
		League: st.League, LastActive: st.LastActive,
	}, nil
}

// GetState returns the user's current gamification snapshot + today's goal.
func (s *Service) GetState(ctx context.Context, userID string) (*StateView, error) {
	st, err := s.repo.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	target := targetFor(st)
	dg, err := s.repo.DailyGoal(ctx, userID, s.today(), target)
	if err != nil {
		return nil, err
	}
	dg.TargetXP = target // always reflect the user's current goal
	dg.Met = dg.EarnedXP >= target
	return &StateView{State: *st, Today: *dg}, nil
}

// applyAward adds XP (and the same amount of gems), advances the streak and
// recomputes the league on a locked state.
func applyAward(st *domain.State, xp int, today time.Time) {
	st.Streak = nextStreak(st.Streak, st.LastActive, today)
	if st.Streak > st.LongestStreak {
		st.LongestStreak = st.Streak
	}
	st.XP += xp
	st.Gems += xp // gems are the spendable currency (earned 1:1 with XP)
	st.League = leagueFor(st.XP)
	st.LastActive = &today
}

// Award adds XP, advances the streak, recomputes the league, and updates today's
// goal — atomically, so the total XP, the daily goal and the weekly league
// (which sums daily goals) always agree. This is the single entry point other
// modules call (via a port) when a learner does something worth rewarding.
func (s *Service) Award(ctx context.Context, userID string, xp int, _ string) error {
	today := s.today()
	_, err := s.repo.Mutate(ctx, userID, domain.Change{
		Apply:   func(st *domain.State) error { applyAward(st, xp, today); return nil },
		DailyXP: xp, Day: today, DefaultTarget: DefaultDailyTarget,
	})
	return err
}

// AwardOnce is Award for a one-time event: the reward is granted at most once
// per user and eventKey (e.g. "lesson:<id>"), even under retries or concurrent
// requests. Reports whether XP was granted by this call.
func (s *Service) AwardOnce(ctx context.Context, userID, eventKey string, xp int, _ string) (bool, error) {
	today := s.today()
	return s.repo.Mutate(ctx, userID, domain.Change{
		Apply:   func(st *domain.State) error { applyAward(st, xp, today); return nil },
		DailyXP: xp, Day: today, DefaultTarget: DefaultDailyTarget,
		EventKey: eventKey, EventXP: xp,
	})
}

// SetDailyTarget stores the user's custom daily XP goal and returns the fresh view.
func (s *Service) SetDailyTarget(ctx context.Context, userID string, target int) (*StateView, error) {
	if target < MinDailyTarget {
		target = MinDailyTarget
	}
	if target > MaxDailyTarget {
		target = MaxDailyTarget
	}
	if _, err := s.repo.Mutate(ctx, userID, domain.Change{Apply: func(st *domain.State) error {
		st.DailyTargetXP = target
		return nil
	}}); err != nil {
		return nil, err
	}
	return s.GetState(ctx, userID)
}

func (s *Service) today() time.Time {
	n := s.clock().UTC()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

// nextStreak advances the streak based on the gap since last activity.
func nextStreak(current int, last *time.Time, today time.Time) int {
	if last == nil {
		return 1
	}
	switch daysBetween(*last, today) {
	case 0: // already active today
		if current < 1 {
			return 1
		}
		return current
	case 1: // consecutive day
		return current + 1
	default: // gap — streak resets
		return 1
	}
}

func daysBetween(a, b time.Time) int {
	a = time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, time.UTC)
	b = time.Date(b.Year(), b.Month(), b.Day(), 0, 0, 0, 0, time.UTC)
	return int(b.Sub(a).Hours() / 24)
}

// Balance returns the user's spendable gem balance.
func (s *Service) Balance(ctx context.Context, userID string) (int, error) {
	st, err := s.repo.Get(ctx, userID)
	if err != nil {
		return 0, err
	}
	return st.Gems, nil
}

// errInsufficientGems is returned when a spend exceeds the balance.
var errInsufficientGems = apperr.New("billing.insufficient_gems", apperr.KindForbidden)

// Spend deducts gems (e.g. to unlock an extra lesson). Errors if insufficient;
// the check and the deduction are one atomic step.
func (s *Service) Spend(ctx context.Context, userID string, amount int) error {
	if amount <= 0 {
		return nil
	}
	_, err := s.repo.Mutate(ctx, userID, domain.Change{Apply: func(st *domain.State) error {
		if st.Gems < amount {
			return errInsufficientGems
		}
		st.Gems -= amount
		return nil
	}})
	return err
}

// Credit adds gems back (e.g. refunding a cancelled booking).
func (s *Service) Credit(ctx context.Context, userID string, amount int) error {
	if amount <= 0 {
		return nil
	}
	_, err := s.repo.Mutate(ctx, userID, domain.Change{Apply: func(st *domain.State) error {
		st.Gems += amount
		return nil
	}})
	return err
}

// --- Daily reward (claimable Duolingo-style chest) ---

const dailyRewardBase = 10

// dailyRewardAmount scales the bonus with the streak (10..30 gems).
func dailyRewardAmount(streak int) int {
	bonus := streak
	if bonus > 10 {
		bonus = 10
	}
	return dailyRewardBase + bonus*2
}

// RewardView is the daily-reward status returned to the delivery layer.
type RewardView struct {
	Available bool
	Amount    int // gems grantable now (or just granted)
	Gems      int // current balance
	Streak    int
}

func (s *Service) claimedToday(st *domain.State, today time.Time) bool {
	return st.LastReward != nil && st.LastReward.Equal(today)
}

// DailyRewardStatus reports whether today's reward is still claimable.
func (s *Service) DailyRewardStatus(ctx context.Context, userID string) (*RewardView, error) {
	st, err := s.repo.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	today := s.today()
	return &RewardView{
		Available: !s.claimedToday(st, today),
		Amount:    dailyRewardAmount(st.Streak),
		Gems:      st.Gems,
		Streak:    st.Streak,
	}, nil
}

var errRewardClaimed = apperr.New("game.reward_already_claimed", apperr.KindConflict)

// ClaimDailyReward grants the daily gem bonus once per UTC day. Idempotent:
// claiming twice the same day returns a conflict instead of double-awarding
// (the check and the grant are one atomic step).
func (s *Service) ClaimDailyReward(ctx context.Context, userID string) (*RewardView, error) {
	today := s.today()
	var view RewardView
	_, err := s.repo.Mutate(ctx, userID, domain.Change{Apply: func(st *domain.State) error {
		if s.claimedToday(st, today) {
			return errRewardClaimed
		}
		amount := dailyRewardAmount(st.Streak)
		st.Gems += amount
		st.LastReward = &today
		view = RewardView{Available: false, Amount: amount, Gems: st.Gems, Streak: st.Streak}
		return nil
	}})
	if err != nil {
		return nil, err
	}
	return &view, nil
}

// dailyMixBonusXP is the once-per-day bonus for finishing the guided daily mix
// (review + shadowing + media). Sized above a single activity so the combined
// session feels like the best XP-per-minute path of the day.
const dailyMixBonusXP = 20

// MixView is the daily-mix status returned to the delivery layer.
type MixView struct {
	Completed bool `json:"completed"`
	BonusXP   int  `json:"bonus_xp"`
	XP        int  `json:"xp"`
}

func (s *Service) mixDoneToday(st *domain.State, today time.Time) bool {
	return st.LastMix != nil && st.LastMix.Equal(today)
}

// DailyMixStatus reports whether today's guided session bonus is still open.
func (s *Service) DailyMixStatus(ctx context.Context, userID string) (*MixView, error) {
	st, err := s.repo.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &MixView{Completed: s.mixDoneToday(st, s.today()), BonusXP: dailyMixBonusXP, XP: st.XP}, nil
}

var errMixIncomplete = apperr.New("game.daily_mix_incomplete", apperr.KindConflict)

// CompleteDailyMix awards the daily-mix bonus once per UTC day. Idempotent:
// a second completion the same day returns the current state without XP.
//
// What the server can observe is required before the bonus: the learner's
// review queue is empty (the mix's review step) and they earned XP today from
// real activity (reviews, shadowing reps, lessons…). The shadowing and media
// steps themselves are self-reported by the client.
func (s *Service) CompleteDailyMix(ctx context.Context, userID string) (*MixView, error) {
	today := s.today()
	st, err := s.repo.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	if s.mixDoneToday(st, today) {
		return &MixView{Completed: true, BonusXP: dailyMixBonusXP, XP: st.XP}, nil
	}
	if s.reviews != nil {
		due, err := s.reviews.DueCount(ctx, userID)
		if err != nil {
			return nil, err
		}
		if due > 0 {
			return nil, errMixIncomplete
		}
	}
	goal, err := s.repo.DailyGoal(ctx, userID, today, targetFor(st))
	if err != nil {
		return nil, err
	}
	if goal.EarnedXP <= 0 {
		return nil, errMixIncomplete
	}
	if _, err := s.repo.Mutate(ctx, userID, domain.Change{
		Apply: func(st *domain.State) error {
			if s.mixDoneToday(st, today) {
				return errMixDone // claimed concurrently: no second bonus
			}
			applyAward(st, dailyMixBonusXP, today)
			st.LastMix = &today
			return nil
		},
		DailyXP: dailyMixBonusXP, Day: today, DefaultTarget: DefaultDailyTarget,
		EventKey: "daily_mix:" + today.Format("2006-01-02"), EventXP: dailyMixBonusXP,
	}); err != nil && !errors.Is(err, errMixDone) {
		return nil, err
	}
	st, err = s.repo.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &MixView{Completed: true, BonusXP: dailyMixBonusXP, XP: st.XP}, nil
}

// errMixDone aborts a daily-mix award that another request already granted.
var errMixDone = errors.New("daily mix already completed")

// NameResolver maps user ids to display names (late-bound to the auth module).
type NameResolver func(ctx context.Context, ids []string) (map[string]string, error)

// SetNames wires the display-name resolver after construction (optional dep).
func (s *Service) SetNames(f NameResolver) { s.names = f }

// weekStart returns the Monday (UTC midnight) of the week containing today.
func (s *Service) weekStart() time.Time {
	today := s.today()
	offset := (int(today.Weekday()) + 6) % 7 // Mon=0 … Sun=6
	return today.AddDate(0, 0, -offset)
}

// LeaderboardRow is one ranked learner in the weekly league.
type LeaderboardRow struct {
	Rank int    `json:"rank"`
	Name string `json:"name"`
	XP   int    `json:"xp"`
	IsMe bool   `json:"is_me"`
}

// LeaderboardView is this week's league standing.
type LeaderboardView struct {
	WeekStart time.Time        `json:"week_start"`
	DaysLeft  int              `json:"days_left"`
	Rows      []LeaderboardRow `json:"rows"`
	MyRank    int              `json:"my_rank"` // 0 = unranked (no XP yet this week)
	MyXP      int              `json:"my_xp"`
}

// Leaderboard ranks learners by XP earned this ISO week (Mon–Sun, UTC). The
// caller is always represented: inside the top rows when ranked high enough,
// appended as a trailing row otherwise, or reported unranked with zero XP.
func (s *Service) Leaderboard(ctx context.Context, userID string, limit int) (*LeaderboardView, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	from := s.weekStart()
	to := from.AddDate(0, 0, 7)
	totals, err := s.repo.WeeklyTotals(ctx, from, to)
	if err != nil {
		return nil, err
	}

	myRank, myXP := 0, 0
	for i, e := range totals {
		if e.UserID == userID {
			myRank, myXP = i+1, e.XP
			break
		}
	}

	top := totals
	if len(top) > limit {
		top = top[:limit]
	}
	ids := make([]string, 0, len(top)+1)
	for _, e := range top {
		ids = append(ids, e.UserID)
	}
	if myRank > limit {
		ids = append(ids, userID)
	}
	names := map[string]string{}
	if s.names != nil {
		if m, err := s.names(ctx, ids); err == nil {
			names = m
		}
	}

	rows := make([]LeaderboardRow, 0, len(top)+1)
	for i, e := range top {
		rows = append(rows, LeaderboardRow{Rank: i + 1, Name: names[e.UserID], XP: e.XP, IsMe: e.UserID == userID})
	}
	if myRank > limit {
		rows = append(rows, LeaderboardRow{Rank: myRank, Name: names[userID], XP: myXP, IsMe: true})
	}

	days := int(to.Sub(s.today()).Hours() / 24)
	return &LeaderboardView{WeekStart: from, DaysLeft: days, Rows: rows, MyRank: myRank, MyXP: myXP}, nil
}

// --- weekly recap --------------------------------------------------------

// Notifier lets the recap job push in-app notifications (notification module).
type Notifier interface {
	Notify(ctx context.Context, userID, ntype string, data map[string]string, link string) error
}

// SetNotifier wires the late-bound notifier.
func (s *Service) SetNotifier(n Notifier) { s.notif = n }

// NotifWeeklyRecap is the notification type for the weekly summary.
const NotifWeeklyRecap = "weekly_recap"

// SendWeeklyRecaps notifies everyone who was active in the *completed* ISO
// week (Mon..Sun UTC) with their XP, active days and league rank — once per
// user per week (persistent dedup, restart-safe). Returns how many were sent.
func (s *Service) SendWeeklyRecaps(ctx context.Context) (int, error) {
	if s.notif == nil {
		return 0, nil
	}
	weekFrom := s.weekStart().AddDate(0, 0, -7) // previous week's Monday
	weekTo := weekFrom.AddDate(0, 0, 7)
	weekKey := weekFrom.Format("2006-01-02")

	totals, err := s.repo.WeeklyTotals(ctx, weekFrom, weekTo)
	if err != nil {
		return 0, err
	}
	sent := 0
	for rank, e := range totals {
		if e.XP <= 0 {
			continue
		}
		done, err := s.repo.RecapSent(ctx, e.UserID, weekKey)
		if err != nil || done {
			continue
		}
		days, _ := s.repo.ActiveDays(ctx, e.UserID, weekFrom, weekTo)
		data := map[string]string{
			"xp":   strconv.Itoa(e.XP),
			"days": strconv.Itoa(days),
			"rank": strconv.Itoa(rank + 1),
		}
		if err := s.notif.Notify(ctx, e.UserID, NotifWeeklyRecap, data, "/league"); err != nil {
			continue // retry next tick; dedup not marked yet
		}
		_ = s.repo.MarkRecapSent(ctx, e.UserID, weekKey)
		sent++
	}
	return sent, nil
}

// StartWeeklyRecaps runs the recap job now and then on the given interval
// until ctx is cancelled (same lifecycle pattern as recording cleanup).
func (s *Service) StartWeeklyRecaps(ctx context.Context, every time.Duration) {
	go func() {
		_, _ = s.SendWeeklyRecaps(ctx)
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_, _ = s.SendWeeklyRecaps(ctx)
			}
		}
	}()
}
