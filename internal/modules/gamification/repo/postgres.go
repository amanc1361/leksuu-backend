//go:build pgx

package repo

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/leksuu/platform/internal/modules/gamification/domain"
)

type PostgresRepo struct{ pool *pgxpool.Pool }

func NewPostgresRepo(pool *pgxpool.Pool) *PostgresRepo { return &PostgresRepo{pool: pool} }

func (r *PostgresRepo) Get(ctx context.Context, userID string) (*domain.State, error) {
	s := domain.State{UserID: userID, League: domain.LeagueBronze}
	err := r.pool.QueryRow(ctx,
		`SELECT xp, gems, streak_days, longest_streak, league, last_active, daily_target_xp, last_reward, last_mix
		   FROM user_gamification WHERE user_id=$1`, userID).
		Scan(&s.XP, &s.Gems, &s.Streak, &s.LongestStreak, &s.League, &s.LastActive, &s.DailyTargetXP, &s.LastReward, &s.LastMix)
	if errors.Is(err, pgx.ErrNoRows) {
		return &s, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *PostgresRepo) Upsert(ctx context.Context, s *domain.State) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO user_gamification (user_id, xp, gems, streak_days, longest_streak, league, last_active, daily_target_xp, last_reward, last_mix, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10, now())
		 ON CONFLICT (user_id) DO UPDATE SET
		   xp=EXCLUDED.xp, gems=EXCLUDED.gems, streak_days=EXCLUDED.streak_days,
		   longest_streak=EXCLUDED.longest_streak, league=EXCLUDED.league,
		   last_active=EXCLUDED.last_active, daily_target_xp=EXCLUDED.daily_target_xp, last_reward=EXCLUDED.last_reward, last_mix=EXCLUDED.last_mix, updated_at=now()`,
		s.UserID, s.XP, s.Gems, s.Streak, s.LongestStreak, s.League, s.LastActive, s.DailyTargetXP, s.LastReward, s.LastMix)
	return err
}

func (r *PostgresRepo) DailyGoal(ctx context.Context, userID string, date time.Time, defaultTarget int) (*domain.DailyGoal, error) {
	g := domain.DailyGoal{Date: date, TargetXP: defaultTarget}
	err := r.pool.QueryRow(ctx,
		`SELECT target_xp, earned_xp, met FROM daily_goals WHERE user_id=$1 AND goal_date=$2`,
		userID, date).Scan(&g.TargetXP, &g.EarnedXP, &g.Met)
	if errors.Is(err, pgx.ErrNoRows) {
		return &g, nil
	}
	if err != nil {
		return nil, err
	}
	return &g, nil
}

func (r *PostgresRepo) AddDailyXP(ctx context.Context, userID string, date time.Time, defaultTarget, xp int) (*domain.DailyGoal, error) {
	g := domain.DailyGoal{Date: date}
	err := r.pool.QueryRow(ctx,
		`INSERT INTO daily_goals (user_id, goal_date, target_xp, earned_xp, met)
		 VALUES ($1,$2,$3,$4, $4 >= $3)
		 ON CONFLICT (user_id, goal_date) DO UPDATE SET
		   earned_xp = daily_goals.earned_xp + EXCLUDED.earned_xp,
		   met = (daily_goals.earned_xp + EXCLUDED.earned_xp) >= daily_goals.target_xp
		 RETURNING target_xp, earned_xp, met`,
		userID, date, defaultTarget, xp).Scan(&g.TargetXP, &g.EarnedXP, &g.Met)
	if err != nil {
		return nil, err
	}
	return &g, nil
}

func (r *PostgresRepo) WeeklyTotals(ctx context.Context, from, to time.Time) ([]domain.WeeklyEntry, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT user_id, COALESCE(SUM(earned_xp),0) AS xp
		   FROM daily_goals
		  WHERE goal_date >= $1 AND goal_date < $2
		  GROUP BY user_id
		 HAVING COALESCE(SUM(earned_xp),0) > 0
		  ORDER BY xp DESC, user_id`,
		from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.WeeklyEntry
	for rows.Next() {
		var e domain.WeeklyEntry
		if err := rows.Scan(&e.UserID, &e.XP); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *PostgresRepo) ActiveDays(ctx context.Context, userID string, from, to time.Time) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM daily_goals WHERE user_id=$1 AND earned > 0 AND day >= $2::date AND day < $3::date`,
		userID, from, to).Scan(&n)
	return n, err
}

func (r *PostgresRepo) RecapSent(ctx context.Context, userID, weekKey string) (bool, error) {
	var one int
	err := r.pool.QueryRow(ctx, `SELECT 1 FROM weekly_recaps WHERE user_id=$1 AND week_key=$2`, userID, weekKey).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (r *PostgresRepo) MarkRecapSent(ctx context.Context, userID, weekKey string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO weekly_recaps (user_id, week_key) VALUES ($1,$2) ON CONFLICT DO NOTHING`, userID, weekKey)
	return err
}
