//go:build pgx

package repo

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/leksuu/platform/internal/modules/shadowing/domain"
)

type PostgresShadowRepo struct{ pool *pgxpool.Pool }

func NewPostgresShadowRepo(pool *pgxpool.Pool) *PostgresShadowRepo {
	return &PostgresShadowRepo{pool: pool}
}

func (r *PostgresShadowRepo) Get(ctx context.Context, userID, itemID string) (*domain.Progress, error) {
	reps := map[string]int{}
	err := r.pool.QueryRow(ctx, `SELECT reps FROM shadowing_progress WHERE user_id=$1 AND item_id=$2`, userID, itemID).Scan(&reps)
	if err != nil {
		// no row -> empty progress
		return &domain.Progress{ItemID: itemID, Reps: map[string]int{}}, nil
	}
	return &domain.Progress{ItemID: itemID, Reps: reps}, nil
}

func (r *PostgresShadowRepo) AddRep(ctx context.Context, userID, itemID, line string) (*domain.Progress, error) {
	reps := map[string]int{}
	err := r.pool.QueryRow(ctx,
		`INSERT INTO shadowing_progress (user_id, item_id, reps)
		   VALUES ($1, $2, jsonb_build_object($3::text, 1))
		 ON CONFLICT (user_id, item_id) DO UPDATE SET
		   reps = jsonb_set(shadowing_progress.reps, ARRAY[$3::text],
		     to_jsonb(COALESCE((shadowing_progress.reps->>$3)::int, 0) + 1), true),
		   updated_at = now()
		 RETURNING reps`,
		userID, itemID, line).Scan(&reps)
	if err != nil {
		return nil, err
	}
	return &domain.Progress{ItemID: itemID, Reps: reps}, nil
}
