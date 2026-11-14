//go:build pgx

package repo

import (
	"context"

	"github.com/leksuu/platform/internal/modules/auth/domain"
)

func (r *PostgresUserRepo) LearningRecords(ctx context.Context, user, kind string) ([]domain.LearningRecord, error) {
	rows, err := r.pool.Query(ctx, `SELECT record_id,value FROM user_learning_records WHERE user_id=$1 AND kind=$2 ORDER BY updated_at,record_id`, user, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.LearningRecord{}
	for rows.Next() {
		var record domain.LearningRecord
		if err := rows.Scan(&record.ID, &record.Value); err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

func (r *PostgresUserRepo) SaveLearningRecord(ctx context.Context, user, kind string, record domain.LearningRecord) error {
	if kind == "daily_mix_step" || kind == "shadow_queue" {
		_, err := r.pool.Exec(ctx, `INSERT INTO user_learning_records(user_id,kind,record_id,value) VALUES($1,$2,$3,$4)
			ON CONFLICT(user_id,kind,record_id) DO NOTHING`, user, kind, record.ID, record.Value)
		return err
	}
	_, err := r.pool.Exec(ctx, `INSERT INTO user_learning_records(user_id,kind,record_id,value) VALUES($1,$2,$3,$4)
		ON CONFLICT(user_id,kind,record_id) DO UPDATE SET
		value=CASE WHEN user_learning_records.value ? 'assessment'
			THEN user_learning_records.value ELSE EXCLUDED.value END,
		updated_at=now()`, user, kind, record.ID, record.Value)
	return err
}
