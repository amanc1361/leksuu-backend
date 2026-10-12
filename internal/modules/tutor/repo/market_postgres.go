//go:build pgx

package repo

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/leksuu/platform/internal/modules/tutor/domain"
)

// Paid sessions: prices, credit and checkouts (domain.MarketStore).

func (r *PostgresTutorRepo) TutorPrice(ctx context.Context, tutorID string) (int, error) {
	var p int
	err := r.pool.QueryRow(ctx, `SELECT session_price_toman FROM tutors WHERE id=$1`, tutorID).Scan(&p)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return p, err
}

func (r *PostgresTutorRepo) SetTutorPrice(ctx context.Context, tutorID string, priceToman int) error {
	_, err := r.pool.Exec(ctx, `UPDATE tutors SET session_price_toman=$2 WHERE id=$1`, tutorID, priceToman)
	return err
}

// ClaimSlot books a free slot in one statement, so two learners can never
// both get it.
func (r *PostgresTutorRepo) ClaimSlot(ctx context.Context, slotID string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `UPDATE tutor_slots SET booked=true WHERE id=$1 AND booked=false`, slotID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (r *PostgresTutorRepo) SaveBookingPrice(ctx context.Context, p domain.BookingPrice) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO booking_prices (booking_id, price_toman, share_percent, paid_via, refunded_toman)
		 VALUES ($1,$2,$3,$4,$5)
		 ON CONFLICT (booking_id) DO UPDATE SET price_toman=$2, share_percent=$3, paid_via=$4, refunded_toman=$5`,
		p.BookingID, p.PriceToman, p.SharePercent, p.PaidVia, p.RefundedToman)
	return err
}

func (r *PostgresTutorRepo) BookingPrices(ctx context.Context, bookingIDs []string) (map[string]domain.BookingPrice, error) {
	out := map[string]domain.BookingPrice{}
	if len(bookingIDs) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx,
		`SELECT booking_id, price_toman, share_percent, paid_via, refunded_toman
		   FROM booking_prices WHERE booking_id = ANY($1)`, bookingIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p domain.BookingPrice
		if err := rows.Scan(&p.BookingID, &p.PriceToman, &p.SharePercent, &p.PaidVia, &p.RefundedToman); err != nil {
			return nil, err
		}
		out[p.BookingID] = p
	}
	return out, rows.Err()
}

func (r *PostgresTutorRepo) SetRefunded(ctx context.Context, bookingID string, refundedToman int) error {
	_, err := r.pool.Exec(ctx, `UPDATE booking_prices SET refunded_toman=$2 WHERE booking_id=$1`, bookingID, refundedToman)
	return err
}

func (r *PostgresTutorRepo) CreditBalance(ctx context.Context, userID string) (int, error) {
	var b int
	err := r.pool.QueryRow(ctx, `SELECT balance_toman FROM learner_credits WHERE user_id=$1`, userID).Scan(&b)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return b, err
}

// AdjustCredit applies a change and its ledger line in one transaction; the
// conditional UPDATE refuses to go below zero, so concurrent debits can't
// overspend.
func (r *PostgresTutorRepo) AdjustCredit(ctx context.Context, userID string, deltaToman int, reason, bookingID string) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `INSERT INTO learner_credits (user_id) VALUES ($1) ON CONFLICT (user_id) DO NOTHING`, userID); err != nil {
		return 0, err
	}
	var bal int
	err = tx.QueryRow(ctx,
		`UPDATE learner_credits SET balance_toman = balance_toman + $2, updated_at = now()
		  WHERE user_id=$1 AND balance_toman + $2 >= 0 RETURNING balance_toman`, userID, deltaToman).Scan(&bal)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, domain.ErrInsufficientCredit
	}
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO credit_ledger (user_id, delta_toman, reason, booking_id) VALUES ($1,$2,$3,$4)`,
		userID, deltaToman, reason, bookingID); err != nil {
		return 0, err
	}
	return bal, tx.Commit(ctx)
}

func (r *PostgresTutorRepo) CreateCheckout(ctx context.Context, c domain.Checkout) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO session_checkouts (id, learner_id, slot_id, price_toman, status, booking_id, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		c.ID, c.LearnerID, c.SlotID, c.PriceToman, c.Status, c.BookingID, c.CreatedAt)
	return err
}

const checkoutCols = `id, learner_id, slot_id, price_toman, status, booking_id, created_at`

func scanCheckout(row pgx.Row) (*domain.Checkout, error) {
	var c domain.Checkout
	err := row.Scan(&c.ID, &c.LearnerID, &c.SlotID, &c.PriceToman, &c.Status, &c.BookingID, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *PostgresTutorRepo) Checkout(ctx context.Context, id string) (*domain.Checkout, error) {
	return scanCheckout(r.pool.QueryRow(ctx, `SELECT `+checkoutCols+` FROM session_checkouts WHERE id=$1`, id))
}

func (r *PostgresTutorRepo) PendingCheckoutForSlot(ctx context.Context, slotID string) (*domain.Checkout, error) {
	return scanCheckout(r.pool.QueryRow(ctx,
		`SELECT `+checkoutCols+` FROM session_checkouts WHERE slot_id=$1 AND status='pending'
		  ORDER BY created_at DESC LIMIT 1`, slotID))
}

func (r *PostgresTutorRepo) SetCheckout(ctx context.Context, id, status, bookingID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE session_checkouts SET status=$2, booking_id=$3 WHERE id=$1`, id, status, bookingID)
	return err
}

var _ domain.MarketStore = (*PostgresTutorRepo)(nil)
