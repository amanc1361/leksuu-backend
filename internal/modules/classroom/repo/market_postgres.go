//go:build pgx

package repo

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/leksuu/platform/internal/modules/classroom/domain"
)

// Group class seats (domain.MarketStore).

func (r *PostgresClassRepo) ClassPrice(ctx context.Context, classID string) (int, error) {
	var p int
	err := r.pool.QueryRow(ctx, `SELECT price_toman FROM class_prices WHERE class_id=$1`, classID).Scan(&p)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return p, err
}

func (r *PostgresClassRepo) SetClassPrice(ctx context.Context, classID string, priceToman int) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO class_prices (class_id, price_toman) VALUES ($1,$2)
		 ON CONFLICT (class_id) DO UPDATE SET price_toman=$2`, classID, priceToman)
	return err
}

func (r *PostgresClassRepo) SaveEnrollmentPrice(ctx context.Context, p domain.EnrollmentPrice) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO class_enrollment_prices (class_id, student_id, price_toman, share_percent, paid_via, refunded_toman)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 ON CONFLICT (class_id, student_id) DO UPDATE SET price_toman=$3, share_percent=$4, paid_via=$5, refunded_toman=$6`,
		p.ClassID, p.StudentID, p.PriceToman, p.SharePercent, p.PaidVia, p.RefundedToman)
	return err
}

const enrollPriceCols = `class_id, student_id, price_toman, share_percent, paid_via, refunded_toman`

func (r *PostgresClassRepo) EnrollmentPrice(ctx context.Context, classID, studentID string) (*domain.EnrollmentPrice, error) {
	var p domain.EnrollmentPrice
	err := r.pool.QueryRow(ctx, `SELECT `+enrollPriceCols+` FROM class_enrollment_prices WHERE class_id=$1 AND student_id=$2`,
		classID, studentID).Scan(&p.ClassID, &p.StudentID, &p.PriceToman, &p.SharePercent, &p.PaidVia, &p.RefundedToman)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *PostgresClassRepo) EnrollmentPrices(ctx context.Context, classIDs []string) (map[string][]domain.EnrollmentPrice, error) {
	out := map[string][]domain.EnrollmentPrice{}
	if len(classIDs) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx, `SELECT `+enrollPriceCols+` FROM class_enrollment_prices WHERE class_id = ANY($1)`, classIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p domain.EnrollmentPrice
		if err := rows.Scan(&p.ClassID, &p.StudentID, &p.PriceToman, &p.SharePercent, &p.PaidVia, &p.RefundedToman); err != nil {
			return nil, err
		}
		out[p.ClassID] = append(out[p.ClassID], p)
	}
	return out, rows.Err()
}

func (r *PostgresClassRepo) SetEnrollmentRefunded(ctx context.Context, classID, studentID string, refunded int) error {
	_, err := r.pool.Exec(ctx, `UPDATE class_enrollment_prices SET refunded_toman=$3 WHERE class_id=$1 AND student_id=$2`,
		classID, studentID, refunded)
	return err
}

func (r *PostgresClassRepo) CreateCheckout(ctx context.Context, c domain.ClassCheckout) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO class_checkouts (id, class_id, student_id, price_toman, status, created_at) VALUES ($1,$2,$3,$4,$5,$6)`,
		c.ID, c.ClassID, c.StudentID, c.PriceToman, c.Status, c.CreatedAt)
	return err
}

func (r *PostgresClassRepo) Checkout(ctx context.Context, id string) (*domain.ClassCheckout, error) {
	var c domain.ClassCheckout
	err := r.pool.QueryRow(ctx, `SELECT id, class_id, student_id, price_toman, status, created_at FROM class_checkouts WHERE id=$1`, id).
		Scan(&c.ID, &c.ClassID, &c.StudentID, &c.PriceToman, &c.Status, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *PostgresClassRepo) HeldSeats(ctx context.Context, classID string, since time.Time) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM class_checkouts WHERE class_id=$1 AND status='pending' AND created_at > $2`, classID, since).Scan(&n)
	return n, err
}

func (r *PostgresClassRepo) SetCheckout(ctx context.Context, id, status string) error {
	_, err := r.pool.Exec(ctx, `UPDATE class_checkouts SET status=$2 WHERE id=$1`, id, status)
	return err
}

var _ domain.MarketStore = (*PostgresClassRepo)(nil)
