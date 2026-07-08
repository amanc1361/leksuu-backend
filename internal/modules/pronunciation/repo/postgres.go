//go:build pgx

package repo

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/leksuu/platform/internal/modules/pronunciation/domain"
)

type PostgresPronRepo struct{ pool *pgxpool.Pool }

func NewPostgresPronRepo(pool *pgxpool.Pool) *PostgresPronRepo {
	return &PostgresPronRepo{pool: pool}
}

func (r *PostgresPronRepo) Upsert(ctx context.Context, userID, itemID string, line, score int) (domain.LineScore, int, error) {
	ls := domain.LineScore{Line: line}
	var prev int
	err := r.pool.QueryRow(ctx,
		`WITH prev AS (
		   SELECT best_score FROM pronunciation_line_scores WHERE user_id=$1 AND item_id=$2 AND line=$3
		 )
		 INSERT INTO pronunciation_line_scores (user_id, item_id, line, best_score, last_score, attempts)
		 VALUES ($1, $2, $3, $4, $4, 1)
		 ON CONFLICT (user_id, item_id, line) DO UPDATE SET
		   last_score = $4,
		   attempts   = pronunciation_line_scores.attempts + 1,
		   best_score = GREATEST(pronunciation_line_scores.best_score, $4),
		   updated_at = now()
		 RETURNING best_score, last_score, attempts, COALESCE((SELECT best_score FROM prev), 0)`,
		userID, itemID, line, score).Scan(&ls.Best, &ls.Last, &ls.Attempts, &prev)
	if err != nil {
		return domain.LineScore{}, 0, err
	}
	return ls, prev, nil
}

func (r *PostgresPronRepo) ByItem(ctx context.Context, userID, itemID string) ([]domain.LineScore, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT line, best_score, last_score, attempts FROM pronunciation_line_scores
		   WHERE user_id=$1 AND item_id=$2 ORDER BY line`, userID, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.LineScore
	for rows.Next() {
		var ls domain.LineScore
		if err := rows.Scan(&ls.Line, &ls.Best, &ls.Last, &ls.Attempts); err != nil {
			return nil, err
		}
		out = append(out, ls)
	}
	return out, rows.Err()
}

func (r *PostgresPronRepo) Summary(ctx context.Context, userID string, masteredThreshold int) (domain.Summary, error) {
	var sum domain.Summary
	err := r.pool.QueryRow(ctx,
		`SELECT
		   count(*),
		   count(*) FILTER (WHERE best_score >= $2),
		   COALESCE(round(avg(best_score)), 0)::int
		 FROM pronunciation_line_scores WHERE user_id=$1`,
		userID, masteredThreshold).Scan(&sum.Scored, &sum.Mastered, &sum.AvgBest)
	if err != nil {
		return domain.Summary{}, err
	}
	return sum, nil
}
