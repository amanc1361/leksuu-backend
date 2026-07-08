//go:build pgx

// Postgres placement store.
package repo

import (
	"context"
	"errors"

	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/leksuu/platform/internal/modules/placement/domain"
)

type PostgresRepo struct{ pool *pgxpool.Pool }

func NewPostgresRepo(pool *pgxpool.Pool) *PostgresRepo { return &PostgresRepo{pool: pool} }

func (r *PostgresRepo) Get(ctx context.Context, userID string) (*domain.Result, error) {
	var res domain.Result
	res.UserID = userID
	err := r.pool.QueryRow(ctx,
		`SELECT cefr, taken_at FROM user_placement WHERE user_id = $1`, userID).
		Scan(&res.CEFR, &res.TakenAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (r *PostgresRepo) Upsert(ctx context.Context, res *domain.Result) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO user_placement (user_id, cefr, taken_at) VALUES ($1,$2,$3)
		 ON CONFLICT (user_id) DO UPDATE SET cefr=EXCLUDED.cefr, taken_at=EXCLUDED.taken_at`,
		res.UserID, res.CEFR, res.TakenAt)
	return err
}
