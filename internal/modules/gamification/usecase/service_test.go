package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/leksuu/platform/internal/modules/gamification/domain"
	"github.com/leksuu/platform/internal/modules/gamification/repo"
	"github.com/leksuu/platform/internal/modules/gamification/usecase"
	"strings"
)

func TestAwardStreakLeagueGoal(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	s := usecase.NewService(repo.NewMemoryRepo(), clock)
	const u = "user-1"

	// Day 1: earn 30 XP.
	if err := s.Award(ctx, u, 30, "lesson"); err != nil {
		t.Fatal(err)
	}
	v, _ := s.GetState(ctx, u)
	if v.State.XP != 30 || v.State.Streak != 1 || v.State.League != domain.LeagueBronze {
		t.Fatalf("after day1: %+v", v.State)
	}
	if v.Today.EarnedXP != 30 || v.Today.Met {
		t.Fatalf("goal day1: %+v", v.Today)
	}

	// Same day again: streak unchanged, goal now met (60 >= 50).
	_ = s.Award(ctx, u, 30, "lesson")
	v, _ = s.GetState(ctx, u)
	if v.State.Streak != 1 || v.State.XP != 60 || !v.Today.Met {
		t.Fatalf("same-day: streak=%d xp=%d met=%v", v.State.Streak, v.State.XP, v.Today.Met)
	}

	// Next day: streak advances to 2, fresh daily goal.
	now = now.AddDate(0, 0, 1)
	_ = s.Award(ctx, u, 10, "lesson")
	v, _ = s.GetState(ctx, u)
	if v.State.Streak != 2 || v.Today.EarnedXP != 10 {
		t.Fatalf("day2: streak=%d earned=%d", v.State.Streak, v.Today.EarnedXP)
	}

	// Gap of several days: streak resets to 1, longest preserved.
	now = now.AddDate(0, 0, 5)
	_ = s.Award(ctx, u, 600, "binge") // also crosses into silver league
	v, _ = s.GetState(ctx, u)
	if v.State.Streak != 1 {
		t.Fatalf("expected streak reset, got %d", v.State.Streak)
	}
	if v.State.LongestStreak != 2 {
		t.Fatalf("expected longest 2, got %d", v.State.LongestStreak)
	}
	if v.State.League != domain.LeagueSilver {
		t.Fatalf("expected silver at %d xp, got %s", v.State.XP, v.State.League)
	}
}

func TestAwardEarnsGems(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s := usecase.NewService(repo.NewMemoryRepo(), func() time.Time { return now })
	u := "gemu"
	if err := s.Award(ctx, u, 30, "lesson"); err != nil {
		t.Fatal(err)
	}
	if err := s.Award(ctx, u, 20, "lesson"); err != nil {
		t.Fatal(err)
	}
	bal, err := s.Balance(ctx, u)
	if err != nil || bal != 50 {
		t.Fatalf("gem balance = %d (err %v), want 50", bal, err)
	}
	if err := s.Spend(ctx, u, 50); err != nil {
		t.Fatalf("spend 50 failed: %v", err)
	}
	if bal, _ := s.Balance(ctx, u); bal != 0 {
		t.Fatalf("after spend bal = %d, want 0", bal)
	}
	if err := s.Spend(ctx, u, 1); err == nil {
		t.Fatal("spend with 0 balance should fail")
	}
}

func TestDailyMixOncePerDay(t *testing.T) {
	now := time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	s := usecase.NewService(repo.NewMemoryRepo(), func() time.Time { return now })
	ctx := context.Background()

	st, err := s.DailyMixStatus(ctx, "u1")
	if err != nil || st.Completed {
		t.Fatalf("fresh status: %+v err=%v", st, err)
	}

	v1, err := s.CompleteDailyMix(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if !v1.Completed || v1.XP != v1.BonusXP {
		t.Fatalf("first completion: %+v", v1)
	}

	// Same day again: no double award.
	v2, err := s.CompleteDailyMix(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if v2.XP != v1.XP {
		t.Fatalf("double-awarded: %d -> %d", v1.XP, v2.XP)
	}

	// Next day: claimable again.
	now = now.Add(24 * time.Hour)
	v3, err := s.CompleteDailyMix(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if v3.XP != v1.XP+v1.BonusXP {
		t.Fatalf("next-day award: %d", v3.XP)
	}
}

func TestWeeklyLeaderboard(t *testing.T) {
	// Wednesday 2026-07-01; week = Mon 2026-06-29 .. Sun 2026-07-05.
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	s := usecase.NewService(repo.NewMemoryRepo(), func() time.Time { return now })
	ctx := context.Background()

	// u1: 30 XP this week; u2: 50; u3: 10; u4 earned last week only.
	mustAward := func(uid string, xp int) {
		if err := s.Award(ctx, uid, xp, "test"); err != nil {
			t.Fatal(err)
		}
	}
	mustAward("u1", 30)
	mustAward("u2", 50)
	mustAward("u3", 10)
	prev := now
	now = time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC) // previous week
	mustAward("u4", 99)
	now = prev

	s.SetNames(func(_ context.Context, ids []string) (map[string]string, error) {
		m := map[string]string{}
		for _, id := range ids {
			m[id] = "name-" + id
		}
		return m, nil
	})

	v, err := s.Leaderboard(ctx, "u3", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Rows) != 3 {
		t.Fatalf("rows=%d (last week must be excluded): %+v", len(v.Rows), v.Rows)
	}
	if v.Rows[0].XP != 50 || v.Rows[1].XP != 30 || v.Rows[2].XP != 10 {
		t.Fatalf("order wrong: %+v", v.Rows)
	}
	if v.Rows[0].Name != "name-u2" || !v.Rows[2].IsMe || v.MyRank != 3 || v.MyXP != 10 {
		t.Fatalf("identity wrong: %+v my=%d/%d", v.Rows, v.MyRank, v.MyXP)
	}
	if v.DaysLeft != 5 { // Wed..Sun inclusive
		t.Fatalf("days_left=%d want 5", v.DaysLeft)
	}

	// Me outside the top rows → appended as a trailing row.
	v2, err := s.Leaderboard(ctx, "u3", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(v2.Rows) != 3 || !v2.Rows[2].IsMe || v2.Rows[2].Rank != 3 {
		t.Fatalf("trailing me row wrong: %+v", v2.Rows)
	}

	// Unranked user (no XP this week).
	v3, _ := s.Leaderboard(ctx, "ghost", 20)
	if v3.MyRank != 0 || v3.MyXP != 0 {
		t.Fatalf("ghost should be unranked: %+v", v3)
	}
}

type recapNotif struct{ got []string }

func (n *recapNotif) Notify(_ context.Context, userID, ntype string, data map[string]string, link string) error {
	n.got = append(n.got, userID+"|"+ntype+"|xp="+data["xp"]+"|days="+data["days"]+"|rank="+data["rank"]+"|"+link)
	return nil
}

func TestWeeklyRecapOncePerUser(t *testing.T) {
	// "Now" is Monday 2026-06-08; the completed week is Jun 1 (Mon) .. Jun 7.
	now := time.Date(2026, 6, 8, 9, 0, 0, 0, time.UTC)
	r := repo.NewMemoryRepo()
	svc := usecase.NewService(r, func() time.Time { return now })
	n := &recapNotif{}
	svc.SetNotifier(n)
	ctx := context.Background()

	// u1 was active 2 days last week (30+20 XP); u2 once (5 XP); u3 only today.
	seed := func(uid string, day time.Time, xp int) {
		if _, err := r.AddDailyXP(ctx, uid, day, 30, xp); err != nil {
			t.Fatal(err)
		}
	}
	seed("u1", time.Date(2026, 6, 2, 10, 0, 0, 0, time.UTC), 30)
	seed("u1", time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC), 20)
	seed("u2", time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC), 5)
	seed("u3", now, 40)

	sent, err := svc.SendWeeklyRecaps(ctx)
	if err != nil || sent != 2 {
		t.Fatalf("sent=%d err=%v, want 2", sent, err)
	}
	joined := ""
	for _, g := range n.got {
		joined += g + "\n"
	}
	if want := "u1|weekly_recap|xp=50|days=2|rank=1|/league"; !contains(joined, want) {
		t.Fatalf("u1 recap wrong:\n%s", joined)
	}
	if want := "u2|weekly_recap|xp=5|days=1|rank=2|/league"; !contains(joined, want) {
		t.Fatalf("u2 recap wrong:\n%s", joined)
	}

	// Idempotent across ticks (persistent dedup).
	sent2, _ := svc.SendWeeklyRecaps(ctx)
	if sent2 != 0 {
		t.Fatalf("second run sent=%d, want 0", sent2)
	}
}

func contains(hay, needle string) bool {
	return len(hay) >= len(needle) && (hay == needle || strings.Contains(hay, needle))
}
