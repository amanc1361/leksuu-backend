//go:build pgx

package repo

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/leksuu/platform/internal/modules/billing/domain"
)

type PostgresBillingRepo struct{ pool *pgxpool.Pool }

func NewPostgresBillingRepo(pool *pgxpool.Pool) *PostgresBillingRepo {
	return &PostgresBillingRepo{pool: pool}
}

func (r *PostgresBillingRepo) GetSubscription(ctx context.Context, userID, product string) (*domain.Subscription, error) {
	if product == "" {
		product = domain.ProductCourses
	}
	var s domain.Subscription
	var plan, status string
	var end time.Time
	err := r.pool.QueryRow(ctx,
		`SELECT plan, status, period_end, updated_at FROM subscriptions WHERE user_id=$1 AND product=$2`, userID, product).
		Scan(&plan, &status, &end, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.UserID = userID
	s.Product = product
	s.Plan = domain.Plan(plan)
	s.Status = status
	s.PeriodEnd = end
	return &s, nil
}

func (r *PostgresBillingRepo) UpsertSubscription(ctx context.Context, s domain.Subscription) error {
	product := s.Product
	if product == "" {
		product = domain.ProductCourses
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO subscriptions (user_id, product, plan, status, period_end, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 ON CONFLICT (user_id, product) DO UPDATE SET plan=$3, status=$4, period_end=$5, updated_at=$6`,
		s.UserID, product, string(s.Plan), s.Status, s.PeriodEnd, s.UpdatedAt)
	return err
}

func (r *PostgresBillingRepo) CreatePayment(ctx context.Context, p domain.Payment) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO payments (authority, user_id, product, plan_id, amount_toman, status, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		p.Authority, p.UserID, p.Product, p.PlanID, p.AmountToman, p.Status, p.CreatedAt)
	return err
}

func (r *PostgresBillingRepo) GetPayment(ctx context.Context, authority string) (*domain.Payment, error) {
	var p domain.Payment
	err := r.pool.QueryRow(ctx,
		`SELECT authority, user_id, product, plan_id, amount_toman, status, COALESCE(ref_id,''), created_at
		 FROM payments WHERE authority=$1`, authority).
		Scan(&p.Authority, &p.UserID, &p.Product, &p.PlanID, &p.AmountToman, &p.Status, &p.RefID, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *PostgresBillingRepo) MarkPayment(ctx context.Context, authority, status, refID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE payments SET status=$2, ref_id=$3 WHERE authority=$1`, authority, status, refID)
	return err
}

func (r *PostgresBillingRepo) UsageToday(ctx context.Context, userID, action, day string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(count,0) FROM daily_usage WHERE user_id=$1 AND day=$2::date AND action=$3`,
		userID, day, action).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return n, err
}

func (r *PostgresBillingRepo) IncrUsage(ctx context.Context, userID, action, day string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`INSERT INTO daily_usage (user_id, day, action, count) VALUES ($1,$2::date,$3,1)
		 ON CONFLICT (user_id, day, action) DO UPDATE SET count = daily_usage.count + 1
		 RETURNING count`,
		userID, day, action).Scan(&n)
	return n, err
}

func (r *PostgresBillingRepo) LessonUnlocked(ctx context.Context, userID, lessonID, today string) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM lesson_unlocks
		   WHERE user_id=$1 AND lesson_id=$2 AND (source='gem' OR (source='free' AND day=$3::date)))`,
		userID, lessonID, today).Scan(&ok)
	return ok, err
}

func (r *PostgresBillingRepo) FreeUnlocksToday(ctx context.Context, userID, today string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM lesson_unlocks WHERE user_id=$1 AND source='free' AND day=$2::date`,
		userID, today).Scan(&n)
	return n, err
}

func (r *PostgresBillingRepo) AddUnlock(ctx context.Context, userID, lessonID, source, day string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO lesson_unlocks (user_id, lesson_id, source, day) VALUES ($1,$2,$3,$4::date)
		 ON CONFLICT (user_id, lesson_id) DO UPDATE SET source=$3, day=$4::date, created_at=now()`,
		userID, lessonID, source, day)
	return err
}

func (r *PostgresBillingRepo) PaidPaymentTotals(ctx context.Context) (map[string]int, int, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT COALESCE(product,'courses'), COALESCE(sum(amount_toman),0), count(*)
		 FROM payments WHERE status='paid' GROUP BY product`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	by := map[string]int{}
	total := 0
	for rows.Next() {
		var product string
		var sum, n int
		if err := rows.Scan(&product, &sum, &n); err != nil {
			return nil, 0, err
		}
		by[product] = sum
		total += n
	}
	return by, total, rows.Err()
}

func (r *PostgresBillingRepo) ActiveSubscriptionCounts(ctx context.Context, now time.Time) (map[string]int, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT COALESCE(product,'courses'), count(*) FROM subscriptions
		 WHERE status='active' AND period_end > $1 GROUP BY product`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var product string
		var n int
		if err := rows.Scan(&product, &n); err != nil {
			return nil, err
		}
		out[product] = n
	}
	return out, rows.Err()
}

func (r *PostgresBillingRepo) ActiveProductsForUsers(ctx context.Context, userIDs []string, now time.Time) (map[string][]string, error) {
	out := map[string][]string{}
	if len(userIDs) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx,
		`SELECT user_id, COALESCE(product,'courses') FROM subscriptions
		 WHERE user_id = ANY($1) AND status='active' AND period_end > $2`, userIDs, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var uid, product string
		if err := rows.Scan(&uid, &product); err != nil {
			return nil, err
		}
		out[uid] = append(out[uid], product)
	}
	return out, rows.Err()
}

func (r *PostgresBillingRepo) PaymentsForUser(ctx context.Context, userID string) ([]domain.Payment, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT authority, user_id, COALESCE(product,'courses'), plan_id, amount_toman, status, COALESCE(ref_id,''), created_at
		   FROM payments WHERE user_id=$1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Payment{}
	for rows.Next() {
		var p domain.Payment
		if err := rows.Scan(&p.Authority, &p.UserID, &p.Product, &p.PlanID, &p.AmountToman, &p.Status, &p.RefID, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *PostgresBillingRepo) SubscriptionsForUser(ctx context.Context, userID string) ([]domain.Subscription, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT user_id, COALESCE(product,'courses'), plan, status, period_end, updated_at
		   FROM subscriptions WHERE user_id=$1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Subscription{}
	for rows.Next() {
		var s domain.Subscription
		if err := rows.Scan(&s.UserID, &s.Product, &s.Plan, &s.Status, &s.PeriodEnd, &s.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *PostgresBillingRepo) ActiveSubscriberIDs(ctx context.Context, now time.Time) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT DISTINCT user_id FROM subscriptions WHERE status='active' AND period_end > $1`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
