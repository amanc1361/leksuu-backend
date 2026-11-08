//go:build pgx

package repo

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/leksuu/platform/internal/modules/course/usecase"
)

func (r *PostgresProgressRepo) LoadMediaProgress(ctx context.Context, user, item, lang string) (usecase.MediaProgress, error) {
	var p usecase.MediaProgress
	var cues []byte
	err := r.pool.QueryRow(ctx, `SELECT position_seconds,attempts,correct_attempts,completed_cue_ids,completed
		FROM media_learner_progress WHERE user_id=$1 AND item_id=$2 AND target_language=$3`, user, item, lang).
		Scan(&p.PositionSeconds, &p.Attempts, &p.CorrectAttempts, &cues, &p.Completed)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	if err = json.Unmarshal(cues, &p.CompletedCueIDs); err != nil {
		return p, err
	}
	return p, nil
}

func (r *PostgresProgressRepo) SaveMediaPosition(ctx context.Context, user, item, lang string, position float64, completed bool) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO media_learner_progress(user_id,item_id,target_language,position_seconds,completed)
		VALUES($1,$2,$3,$4,$5) ON CONFLICT(user_id,item_id,target_language) DO UPDATE
		SET position_seconds=EXCLUDED.position_seconds,completed=media_learner_progress.completed OR EXCLUDED.completed,updated_at=now()`,
		user, item, lang, position, completed)
	return err
}

func (r *PostgresProgressRepo) RecordMediaAttempt(ctx context.Context, user, item, lang, cue string, passed bool) (usecase.MediaProgress, error) {
	var p usecase.MediaProgress
	var cues []byte
	err := r.pool.QueryRow(ctx, `INSERT INTO media_learner_progress(user_id,item_id,target_language,attempts,correct_attempts,completed_cue_ids)
		VALUES($1,$2,$3,1,CASE WHEN $5 THEN 1 ELSE 0 END,CASE WHEN $5 THEN jsonb_build_array($4::text) ELSE '[]'::jsonb END)
		ON CONFLICT(user_id,item_id,target_language) DO UPDATE SET
		attempts=media_learner_progress.attempts+1,
		correct_attempts=media_learner_progress.correct_attempts+CASE WHEN $5 THEN 1 ELSE 0 END,
		completed_cue_ids=CASE WHEN NOT $5 OR media_learner_progress.completed_cue_ids ? $4 THEN media_learner_progress.completed_cue_ids
		ELSE media_learner_progress.completed_cue_ids || jsonb_build_array($4::text) END,updated_at=now()
		RETURNING position_seconds,attempts,correct_attempts,completed_cue_ids,completed`, user, item, lang, cue, passed).
		Scan(&p.PositionSeconds, &p.Attempts, &p.CorrectAttempts, &cues, &p.Completed)
	if err != nil {
		return p, err
	}
	if err = json.Unmarshal(cues, &p.CompletedCueIDs); err != nil {
		return p, err
	}
	return p, nil
}
